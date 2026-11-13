package handler

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Failure tests (S20): the engine keeps no state in memory and its writes are idempotent, so any
// interruption is repaired by the next advance or sweep, and nothing is created twice.

// duplicateTasks counts (step, task, round) triples of a request that have more than one issue.
func duplicateTasks(t *testing.T, request db.Issue) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM (
		SELECT 1 FROM issue WHERE parent_issue_id = $1 AND flow_task IS NOT NULL
		GROUP BY flow_step, flow_task, flow_round HAVING count(*) > 1) d`, request.ID)
}

func resetSweepCursor() {
	flowSweepCursor.Lock()
	flowSweepCursor.workspace, flowSweepCursor.id = pgtype.UUID{}, pgtype.UUID{}
	flowSweepCursor.Unlock()
}

func TestChaos_CrashBetweenPhase1And2(t *testing.T) {
	f, request := autoFixture(t)
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	// Phase 1 committed (the request is at draft) and the process died before phase 2 made the tasks.
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id IN (SELECT id FROM issue WHERE parent_issue_id = $1 AND flow_step = 'draft')`, request.ID)
	dbfx.Exec(t, `DELETE FROM issue WHERE parent_issue_id = $1 AND flow_step = 'draft'`, request.ID)
	if n := len(tasksAt(t, request, "draft", 0)); n != 0 {
		t.Fatalf("setup: %d draft tasks left", n)
	}

	sweep(t)

	if draft := tasksAt(t, request, "draft", 0); len(draft) != 1 || draft["crosscheck"] == "" {
		t.Fatalf("draft tasks = %v, want the missing task made once", draft)
	}
	sweep(t)
	advance(t, request)
	if n := duplicateTasks(t, request); n != 0 {
		t.Fatalf("%d duplicated tasks after repeated passes", n)
	}
	if n := countTasks(t, request); n != 2 {
		t.Fatalf("task issues = %d, want 2 (docs and crosscheck)", n)
	}
}

func TestChaos_RestartHandlerMidStep(t *testing.T) {
	f, request := autoFixture(t)
	// A step with two tasks was half made when the process died.
	finishTask(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, request)
	finishTask(t, tasksAt(t, request, "draft", 0)["crosscheck"], f.Agents.Drafter, "")
	advance(t, request)
	gates := tasksAt(t, request, "gate", 0)
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, gates["customs_gate"])
	dbfx.Exec(t, `DELETE FROM issue WHERE id = $1`, gates["customs_gate"])

	// A restart forgets where the sweep was.
	resetSweepCursor()
	sweep(t)

	if got := tasksAt(t, request, "gate", 0); len(got) != 2 {
		t.Fatalf("gate tasks = %v, want both after the restart", got)
	}
	if n := duplicateTasks(t, request); n != 0 {
		t.Fatalf("%d duplicated tasks", n)
	}
}

func TestChaos_SweepAndEventConcurrent(t *testing.T) {
	f, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	withFeatureFlag(t, testHandler, "flow_runner", false)
	closeTaskForEvent(t, docs, f.Agents.Docs, "")
	withFeatureFlag(t, testHandler, "flow_runner", true)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); testHandler.advanceFlowRequests(context.Background(), request.ID) }()
		go func() { defer wg.Done(); _, _ = testHandler.SweepFlowRequests(context.Background()) }()
		go func() {
			defer wg.Done()
			_, _ = testHandler.AdvanceFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)))
		}()
	}
	wg.Wait()

	if n := duplicateTasks(t, request); n != 0 {
		t.Fatalf("%d duplicated tasks after concurrent passes", n)
	}
	if got := requestStatus(t, request); got != "draft" {
		t.Fatalf("status = %q, want draft", got)
	}
	if draft := tasksAt(t, request, "draft", 0); len(draft) != 1 {
		t.Fatalf("draft tasks = %v, want one", draft)
	}
}

func TestChaos_HumanMovesRequestDuringRun(t *testing.T) {
	_, request := autoFixture(t)
	id := uuidToString(request.ID)
	docs := tasksAt(t, request, "collect", 0)["docs"]

	// A member who manages the squad drags the request on while the collect task still runs (B8).
	updateIssueForTest(t, id, map[string]any{"status": "draft"}).Want(http.StatusOK)

	if status, _, _ := issueStatusAndAssignee(t, docs); status != "cancelled" {
		t.Fatalf("the collect task is %q after the move, want cancelled", status)
	}
	state, _ := flowRequestState(loadIssueRow(t, id))
	if state.Step != "draft" {
		t.Fatalf("flow step = %q, want draft", state.Step)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_manual_move'`, request.ID); n != 1 {
		t.Fatalf("flow_manual_move entries = %d, want 1", n)
	}
	sweep(t)
	if draft := tasksAt(t, request, "draft", 0); len(draft) != 1 {
		t.Fatalf("draft tasks = %v, want the step the member chose opened once", draft)
	}
}

func TestChaos_TaskCancelledByMember(t *testing.T) {
	f, request := autoFixture(t)
	finishTask(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, request)
	finishTask(t, tasksAt(t, request, "draft", 0)["crosscheck"], f.Agents.Drafter, "")
	advance(t, request)
	gates := tasksAt(t, request, "gate", 0)

	// A member cancels a gate: it has no verdict, so it counts as a rejection and the draft goes back.
	updateIssueForTest(t, gates["docs_gate"], map[string]any{"status": "cancelled"}).Want(http.StatusOK)
	endRuns(t, gates["docs_gate"])
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "approved")
	st := advance(t, request).State
	if st.Step != "draft" || st.Rework != 1 {
		t.Fatalf("step %q, rework %d; want the draft back for a fix", st.Step, st.Rework)
	}
}

func TestChaos_RunFailureBlocksTask(t *testing.T) {
	_, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	// A run that fails for good lands its task on blocked, with no verdict: a system write.
	endRuns(t, docs)
	dbfx.Exec(t, `UPDATE issue SET status = 'blocked' WHERE id = $1`, docs)

	sweep(t)

	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	if state.Stop != issueworkflow.Stop1 || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %v, status %q; want blocked at stop 1", state.Stop, requestStatus(t, request))
	}
}

func TestChaos_ArchivedAgent(t *testing.T) {
	f, request := autoFixture(t)
	// The drafter is archived before the draft step opens: the runner stalls the request, not loops.
	dbfx.Exec(t, `UPDATE agent SET archived_at = now() WHERE id = $1`, f.Agents.Drafter)
	t.Cleanup(func() { dbfx.Exec(t, `UPDATE agent SET archived_at = NULL WHERE id = $1`, f.Agents.Drafter) })
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	sweep(t)

	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	if state.Stop != issueworkflow.StopStalled || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %v, status %q; want stalled", state.Stop, requestStatus(t, request))
	}
	if n := countTasks(t, request); n != 1 {
		t.Fatalf("task issues = %d, want only the collect task", n)
	}
}
