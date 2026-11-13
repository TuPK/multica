package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Runner tests (S18 to S21 of docs/engineering/kitviet-workflow-plan.md). The runner is the same
// engine the lead calls, started by a sub-issue event, the end of a run or the sweep, for a request
// whose workflow has flow_auto set while FF_FLOW_RUNNER is on.

// setFlowAuto turns the runner on or off for a workflow, the way the admin endpoint does.
func setFlowAuto(t *testing.T, workflowID string, auto bool) {
	t.Helper()
	dbfx.Exec(t, `UPDATE issue_workflow SET flow_auto = $2 WHERE id = $1`, workflowID, auto)
}

// autoFixture is the Trade flow with the runner on, and a request planned and waiting at collect.
func autoFixture(t *testing.T) (tradeFlowFixture, db.Issue) {
	t.Helper()
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	return f, request
}

// closeTaskForEvent closes a task the way its agent does, after its run has ended, so the write is
// the event the runner reacts to.
func closeTaskForEvent(t *testing.T, issueID, agentID, outcome string) {
	t.Helper()
	endRuns(t, issueID)
	attachAs(t, issueID, agentID)
	body := map[string]any{"status": "done"}
	if outcome != "" {
		body["outcome"] = outcome
	}
	updateIssueForTest(t, issueID, body).Want(http.StatusOK)
}

func TestChildEvents_AdvancesAutoRequest(t *testing.T) {
	f, request := autoFixture(t)

	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")

	if got := requestStatus(t, request); got != "draft" {
		t.Fatalf("request status = %q after its collect task closed, want draft with no advance call", got)
	}
	if draft := tasksAt(t, request, "draft", 0); len(draft) != 1 || draft["crosscheck"] == "" {
		t.Fatalf("draft tasks = %v, want crosscheck opened by the runner", draft)
	}
}

func TestChildEvents_ManualFlowIsNotAdvanced(t *testing.T) {
	f, request := autoFixture(t)
	setFlowAuto(t, f.Workflow.ID, false)

	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")

	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("request status = %q, want collect: the lead advances a flow that is not auto", got)
	}
}

func TestChildDone_NotDispatchedForAutoRequest(t *testing.T) {
	f, request := autoFixture(t)
	id := uuidToString(request.ID)

	// Between steps the lead gets no wake-up: not at the end of collect, not at the end of draft.
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	closeTaskForEvent(t, tasksAt(t, request, "draft", 0)["crosscheck"], f.Agents.Drafter, "")
	if got := requestStatus(t, request); got != "gate" {
		t.Fatalf("request status = %q, want gate", got)
	}
	if runs := childDoneRuns(t, id); len(runs) != 0 {
		t.Fatalf("the lead was woken %d times between steps: %+v", len(runs), runs)
	}
}

func TestChildDone_StillFiresForManualFlow(t *testing.T) {
	f, request := autoFixture(t)
	setFlowAuto(t, f.Workflow.ID, false)

	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")

	if runs := childDoneRuns(t, uuidToString(request.ID)); len(runs) != 1 {
		t.Fatalf("child_done runs = %+v, want one for the lead of a flow that is not auto", runs)
	}
}

// Detach with cancel_tasks takes the request off its flow and cancels its open tasks. The lead must
// not be woken for those cancelled sub-issues: nothing runs for a detached request.
func TestDetachCancelTasks_DoesNotWakeTheLead(t *testing.T) {
	f, request := autoFixture(t)
	setFlowAuto(t, f.Workflow.ID, false)
	id := uuidToString(request.ID)
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, request)
	if got := requestStatus(t, request); got != "draft" {
		t.Fatalf("request status = %q, want draft", got)
	}
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, id)
	before := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE context->>'wakeup_system' = 'child_done' AND issue_id = $1`, id)
	crosscheck := tasksAt(t, request, "draft", 0)["crosscheck"]
	if queuedRuns(t, crosscheck) == 0 {
		t.Fatal("the open task has no run to cancel; the test would prove nothing")
	}

	if err := testHandler.DetachFlow(context.Background(), loadIssueRow(t, id), true); err != nil {
		t.Fatalf("DetachFlow: %v", err)
	}
	runWakeupTick(t)
	runWakeupTick(t)

	if runs := childDoneRuns(t, id); len(runs) != before {
		t.Fatalf("the lead was woken %d times after the detach: %+v", len(runs)-before, runs)
	}
	if n := queuedRuns(t, crosscheck); n != 0 {
		t.Fatalf("the cancelled task still has %d active runs", n)
	}
}

func TestChildDone_ResumesWhenFlagOff(t *testing.T) {
	f, request := autoFixture(t)
	withFeatureFlag(t, testHandler, "flow_runner", false)

	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")

	if runs := childDoneRuns(t, uuidToString(request.ID)); len(runs) != 1 {
		t.Fatalf("child_done runs = %+v, want one once the runner flag is off", runs)
	}
	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("request status = %q, want collect: nothing advances it with the flag off", got)
	}
}

// ---- the sweep (S19) ----

// backdateStep makes the request enter its current step the given time ago.
func backdateStep(t *testing.T, request db.Issue, ago time.Duration) {
	t.Helper()
	dbfx.Exec(t, `UPDATE issue SET flow = jsonb_set(flow, '{entered_at}', to_jsonb($2::text)) WHERE id = $1`,
		request.ID, time.Now().Add(-ago).UTC().Format(time.RFC3339))
}

// backdateRuns moves every run of an issue into the past.
func backdateRuns(t *testing.T, issueID string, ago time.Duration) {
	t.Helper()
	dbfx.Exec(t, `UPDATE agent_task_queue SET created_at = now() - $2::interval,
		started_at = CASE WHEN started_at IS NOT NULL THEN now() - $2::interval END,
		completed_at = CASE WHEN completed_at IS NOT NULL THEN now() - $2::interval END
		WHERE issue_id = $1`, issueID, fmt.Sprintf("%d seconds", int(ago.Seconds())))
}

func sweep(t *testing.T) int {
	t.Helper()
	n, err := testHandler.SweepFlowRequests(context.Background())
	if err != nil {
		t.Fatalf("SweepFlowRequests: %v", err)
	}
	return n
}

func queuedRuns(t *testing.T, issueID string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND status IN ('queued', 'dispatched', 'running')`, issueID)
}

func TestFlowSweep_AdvancesMissedEvent(t *testing.T) {
	f, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	// The close happens while the runner is off, so no event moves the request.
	withFeatureFlag(t, testHandler, "flow_runner", false)
	closeTaskForEvent(t, docs, f.Agents.Docs, "")
	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("status = %q, want collect before the sweep", got)
	}

	withFeatureFlag(t, testHandler, "flow_runner", true)
	sweep(t)

	if got := requestStatus(t, request); got != "draft" {
		t.Fatalf("status = %q after the sweep, want draft", got)
	}
}

func TestFlowSweep_StartsUnstartedEligibleRequest(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	id := dbfx.Issue(t, "Trade request unstarted "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": f.SquadID,
	})
	if _, ok := flowRequestState(loadIssueRow(t, id)); ok {
		t.Fatal("the fixture request already has a flow")
	}

	sweep(t)

	state, ok := flowRequestState(loadIssueRow(t, id))
	if !ok || state.Step != "todo" {
		t.Fatalf("flow state = %+v (ok %v), want the request started at the plan step", state, ok)
	}
}

// Detach is a person's decision to take a request off its flow. The sweep starts requests that
// missed the engine, and a detached request looks like one: it must not be put back.
func TestFlowSweep_DoesNotRestartADetachedRequest(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	request := f.newRequest(t)
	id := uuidToString(request.ID)
	if err := testHandler.DetachFlow(context.Background(), loadIssueRow(t, id), true); err != nil {
		t.Fatalf("DetachFlow: %v", err)
	}

	sweep(t)

	if state, ok := flowRequestState(loadIssueRow(t, id)); ok {
		t.Fatalf("the sweep put the detached request back on its flow: %+v", state)
	}
}

// The company planning job files its issues in the name of the squad's leader and gives them to
// the squad. They are not shipment requests: the sweep must not put them on the squad's flow, which
// would wake the lead on work the flow cannot plan.
func TestFlowSweep_LeavesAgentFiledIssuesOffTheFlow(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	id := dbfx.Issue(t, "Trade request planning "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": f.SquadID,
		"creator_type": "agent", "creator_id": f.Agents.Docs,
	})

	sweep(t)

	if state, ok := flowRequestState(loadIssueRow(t, id)); ok {
		t.Fatalf("the sweep put an agent-filed issue on the flow: %+v", state)
	}
}

func TestFlowSweep_IdleTaskRetriesThenStalls(t *testing.T) {
	_, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	// The first run ended long ago without closing the task.
	endRuns(t, docs)
	backdateStep(t, request, 5*time.Hour)
	backdateRuns(t, docs, 3*time.Hour)

	sweep(t)
	if n := queuedRuns(t, docs); n != 1 {
		t.Fatalf("queued runs on the idle task = %d, want a retry", n)
	}
	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("status = %q after the retry, want collect", got)
	}

	// The retry also ends without closing it: two attempts are used.
	endRuns(t, docs)
	backdateRuns(t, docs, 3*time.Hour)
	sweep(t)

	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	if requestStatus(t, request) != "blocked" || state.Stop != issueworkflow.StopStalled {
		t.Fatalf("status %q, stop %v; want blocked, stalled in one pass", requestStatus(t, request), state.Stop)
	}
	if status, _, _ := issueStatusAndAssignee(t, docs); status != "blocked" || issueOutcome(t, docs) != "blocked" {
		t.Fatalf("the stuck task is %q with outcome %q, want blocked", status, issueOutcome(t, docs))
	}
	if !containsAll(lastComment(t, request), "BLOCKED · stalled") {
		t.Fatalf("stalled comment = %q", lastComment(t, request))
	}
}

// The brief of a retried run says what that run must do. A task is closed with the commands in its
// description; the lead of a request has no task to close, it plans.
func TestFlowSweep_RetryBriefNamesTheJobOfTheRun(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	request := f.newRequest(t)
	id := uuidToString(request.ID)
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, id)
	backdateStep(t, request, 6*time.Hour)
	sweep(t)
	leadNote := queuedRunNote(t, id)
	if !containsAll(leadNote, "flow plan") || containsAll(leadNote, "close it") {
		t.Fatalf("the lead's retry brief = %q, want it to ask for the plan and not to close a task", leadNote)
	}

	f.plan(t, request, nil)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	endRuns(t, docs)
	backdateStep(t, request, 5*time.Hour)
	backdateRuns(t, docs, 3*time.Hour)
	sweep(t)
	if taskNote := queuedRunNote(t, docs); !containsAll(taskNote, "close it") {
		t.Fatalf("the task's retry brief = %q, want it to ask to close the task", taskNote)
	}
}

func queuedRunNote(t *testing.T, issueID string) string {
	t.Helper()
	var note string
	dbfx.QueryRow(t, `SELECT COALESCE(handoff_note, '') FROM agent_task_queue
		WHERE issue_id = $1 AND status = 'queued' ORDER BY created_at DESC LIMIT 1`, issueID).Scan(&note)
	return note
}

func TestFlowSweep_BlockedAgentTaskGetsRunWhenReady(t *testing.T) {
	_, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	// The task was opened while its agent could not take a run: it has none.
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, docs)

	sweep(t)

	if n := queuedRuns(t, docs); n != 1 {
		t.Fatalf("queued runs = %d, want the sweep to give the task a run", n)
	}
}

func TestFlowSweep_HardCapCancelsRun(t *testing.T) {
	f, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, docs)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, f.Agents.Docs).Scan(&runtimeID)
	run := dbfx.Task(t, f.Agents.Docs, testutil.Cols{"runtime_id": runtimeID, "issue_id": docs, "status": "running",
		"started_at": time.Now().Add(-5 * time.Hour)})

	sweep(t)

	var status string
	dbfx.QueryRow(t, `SELECT status FROM agent_task_queue WHERE id = $1`, run).Scan(&status)
	if status != "cancelled" {
		t.Fatalf("run status = %q, want cancelled past the hard cap", status)
	}
	if got := requestStatus(t, request); got == "blocked" {
		t.Fatal("one cancelled run stalled the request; two attempts are allowed")
	}
}

func TestFlowSweep_LeadNoPlanWakesThenStalls(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	request := f.newRequest(t)
	id := uuidToString(request.ID)
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, id)
	backdateStep(t, request, 6*time.Hour)

	sweep(t)
	if n := queuedRuns(t, id); n != 1 {
		t.Fatalf("lead runs = %d, want the lead woken once", n)
	}
	for i := 0; i < 2; i++ {
		endRuns(t, id)
		backdateRuns(t, id, 5*time.Hour)
		sweep(t)
	}
	state, _ := flowRequestState(loadIssueRow(t, id))
	if state.Stop != issueworkflow.StopStalled || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %v, status %q; want the request stalled after the lead's attempts", state.Stop, requestStatus(t, request))
	}
}

// A person answers a stop and presses Resume. The answer woke the lead, and Resume then moved the
// step's entered_at past that run's creation. The run is still going, so the sweep must not give the
// lead a second one.
func TestFlowSweep_LeadRunStartedBeforeResumeIsNotDoubled(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	request := f.newRequest(t)
	id := uuidToString(request.ID)
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, id)
	backdateStep(t, request, 6*time.Hour)

	sweep(t)
	if n := queuedRuns(t, id); n != 1 {
		t.Fatalf("lead runs = %d, want the lead woken once", n)
	}
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'running', started_at = now() - interval '25 seconds',
		created_at = now() - interval '30 seconds' WHERE issue_id = $1`, id)
	backdateStep(t, request, 0)

	sweep(t)

	if n := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, id); n != 1 {
		t.Fatalf("runs = %d, want 1: a lead run that began before Resume is still the lead's run", n)
	}
}

func TestFlowRunner_FlagOffIsNoop(t *testing.T) {
	f, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	withFeatureFlag(t, testHandler, "flow_runner", false)
	closeTaskForEvent(t, docs, f.Agents.Docs, "")
	backdateStep(t, request, 5*time.Hour)

	if n := sweep(t); n != 0 {
		t.Fatalf("the sweep changed %d requests with the flag off", n)
	}
	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("status = %q, want collect", got)
	}
	// The lead can still advance by hand.
	if resp := advance(t, request); resp.State.Step != "draft" {
		t.Fatalf("a manual advance reached %q, want draft", resp.State.Step)
	}
}

func TestFlowRunner_RequiresWorkflowAuto(t *testing.T) {
	f, request := autoFixture(t)
	withFeatureFlag(t, testHandler, "flow_runner", false)
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	withFeatureFlag(t, testHandler, "flow_runner", true)
	setFlowAuto(t, f.Workflow.ID, false)

	sweep(t)

	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("status = %q, want collect: a flow that is not auto is not swept", got)
	}
}

func TestChildEvents_FailedAdvanceLeavesSweepToRetry(t *testing.T) {
	f, request := autoFixture(t)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	withFeatureFlag(t, testHandler, "flow_runner", false)
	closeTaskForEvent(t, docs, f.Agents.Docs, "")
	withFeatureFlag(t, testHandler, "flow_runner", true)

	// The event handler fails (its context is gone): nothing is half done.
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	testHandler.advanceFlowRequests(gone, request.ID)
	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("status = %q after a failed advance, want collect", got)
	}

	sweep(t)
	if got := requestStatus(t, request); got != "draft" {
		t.Fatalf("status = %q after the sweep, want draft", got)
	}
}

// B7: a pass over a full page of requests finishes inside the job's run timeout.
func TestFlowSweep_Handles200InBudget(t *testing.T) {
	withFeatureFlag(t, testHandler, "flow_runner", true)
	f := newRehearsalFixture(t)
	setFlowAuto(t, f.Workflow.ID, true)
	flow := fmt.Sprintf(`{"workflow_id":"%s","step":"blocked","rework":0,"stop":1,"blocked_from":"collect"}`, f.Workflow.ID)
	dbfx.Exec(t, `INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, position, number, flow)
		SELECT $1, 'sweep load ' || g, 'blocked', 'none', 'member', $2, 0,
		       (SELECT COALESCE(MAX(number), 0) FROM issue WHERE workspace_id = $1) + g, $3::jsonb
		FROM generate_series(1, 200) AS g`, testWorkspaceID, testUserID, flow)
	dbfx.Exec(t, `UPDATE workspace w SET issue_counter = GREATEST(w.issue_counter, (SELECT MAX(number) FROM issue WHERE workspace_id = w.id)) WHERE id = $1`, testWorkspaceID)

	started := time.Now()
	sweep(t)
	if took := time.Since(started); took > 45*time.Second {
		t.Fatalf("a pass over 200 requests took %v, over the 45s run timeout", took)
	}
}
