package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Reproduction for the stuck task of the Kit Viet copy-flow runs: a daemon restart failed every run of a
// task, the automatic retries were used up, and the task stayed in todo with no run and no stop code.
// `multica issue flow advance` answered 200 with a noop and nothing else happened until a comment on the
// task issue queued a run. The sweep would have caught it, but only for a flow with flow_auto set and the
// runner on; a request the lead moves has no sweep for its agent tasks.

// failRunsLikeDaemonRestart ends every run of the task the way RecoverOrphanedTasksForRuntime does, and
// adds failed runs until the task has attempts of them.
func failRunsLikeDaemonRestart(t *testing.T, taskID, agentID string, attempts int) {
	t.Helper()
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, taskID)
	for i := 0; i < attempts; i++ {
		dbfx.Task(t, agentID, testutil.Cols{
			"issue_id": taskID, "runtime_id": runtimeID, "status": "failed",
			"error": "daemon restarted while task was in flight", "failure_reason": "runtime_recovery",
			"completed_at": testutil.Raw("now()"),
		})
	}
}

// manualFlowFixture is the Trade flow with the runner off (flow_auto false, the default), planned and
// waiting at collect.
func manualFlowFixture(t *testing.T) (tradeFlowFixture, string) {
	t.Helper()
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	return f, uuidToString(request.ID)
}

// advanceAsManager is `multica issue flow advance`: the request of a member who manages the squad.
func advanceAsManager(t *testing.T, c flowCaller) FlowAdvanceResponse {
	t.Helper()
	var resp FlowAdvanceResponse
	c.call("manager", testHandler.AdvanceIssueFlow, "POST", "/advance", map[string]any{}).Want(http.StatusOK).JSON(&resp)
	return resp
}

func TestAdvance_GivesARunToAnAgentTaskWithNoLiveRun(t *testing.T) {
	f, requestID := manualFlowFixture(t)
	request := loadIssueRow(t, requestID)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	// One run failed with the daemon restart; the second attempt is still allowed.
	failRunsLikeDaemonRestart(t, docs, f.Agents.Docs, 1)
	if n := queuedRuns(t, docs); n != 0 {
		t.Fatalf("setup: %d live runs, want none", n)
	}

	c := newFlowCaller(t, f, request)
	resp := advanceAsManager(t, c)

	if n := queuedRuns(t, docs); n != 1 {
		t.Fatalf("live runs on the task after advance = %d, want 1: advance answered %v and left the task without a run", n, actionKinds(resp))
	}
	if len(resp.Actions) != 1 || resp.Actions[0].Kind != "redispatched_tasks" || len(resp.Actions[0].Tasks) != 1 || resp.Actions[0].Tasks[0] != "docs" {
		t.Fatalf("actions = %+v, want one redispatched_tasks naming docs", resp.Actions)
	}
	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("request status = %q, want collect", got)
	}

	// The advance is idempotent: the task has a live run now.
	again := advanceAsManager(t, c)
	if kinds := actionKinds(again); len(kinds) != 1 || kinds[0] != "noop" {
		t.Fatalf("second advance = %v, want noop", kinds)
	}
	if n := queuedRuns(t, docs); n != 1 {
		t.Fatalf("live runs after the second advance = %d, want still 1", n)
	}
}

func TestAdvance_StopsARequestWhoseAgentTaskUsedAllItsAttempts(t *testing.T) {
	f, requestID := manualFlowFixture(t)
	request := loadIssueRow(t, requestID)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	failRunsLikeDaemonRestart(t, docs, f.Agents.Docs, issueworkflow.DefaultTaskMaxAttempts)

	resp := advanceAsManager(t, newFlowCaller(t, f, request))

	if len(resp.Actions) != 1 || resp.Actions[0].Kind != "stopped" || resp.Actions[0].Code != "flow_task_stuck" {
		t.Fatalf("actions = %+v, want one stopped with code flow_task_stuck", resp.Actions)
	}
	if state, _ := flowRequestState(loadIssueRow(t, requestID)); state.Stop != issueworkflow.StopStalled {
		t.Fatalf("stop = %v, want stalled", state.Stop)
	}
	if got := requestStatus(t, request); got != "blocked" {
		t.Fatalf("request status = %q, want blocked", got)
	}
	if status, _, _ := issueStatusAndAssignee(t, docs); status != "blocked" {
		t.Fatalf("the exhausted task is %q, want blocked", status)
	}
	if n := queuedRuns(t, docs); n != 0 {
		t.Fatalf("a stopped request got %d new runs", n)
	}
}

func TestAdvance_LeavesATaskWithALiveRunAlone(t *testing.T) {
	f, requestID := manualFlowFixture(t)
	request := loadIssueRow(t, requestID)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	// The run the engine queued when it opened the task is live.
	if n := queuedRuns(t, docs); n != 1 {
		t.Fatalf("setup: %d live runs on the new task, want 1", n)
	}

	resp := advanceAsManager(t, newFlowCaller(t, f, request))

	if kinds := actionKinds(resp); len(kinds) != 1 || kinds[0] != "noop" {
		t.Fatalf("advance with a live run = %v, want noop", kinds)
	}
	if n := queuedRuns(t, docs); n != 1 {
		t.Fatalf("live runs = %d, want still 1", n)
	}
}
