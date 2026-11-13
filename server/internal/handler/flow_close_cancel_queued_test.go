package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A flow task that closes while a run of it is still queued must not keep its
// request waiting: the engine counts the task open until the run is gone. The
// queued run is cancelled; a run that already started is left to finish, since
// its agent may still be writing the verdict.
func TestClosedFlowTaskCancelsItsQueuedRuns(t *testing.T) {
	ctx := context.Background()
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{"switches": map[string]any{"customs_lookup": true}}).Want(http.StatusOK)

	docs := tasksAt(t, request, "collect", 0)["docs"]
	// Opening the collect step already queued the docs agent's run of this task; that is the queued run.
	var queued string
	dbfx.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued' ORDER BY created_at DESC LIMIT 1`, docs, f.Agents.Docs).Scan(&queued)
	// The schema allows one pending run per (issue, agent): the running run belongs to another agent.
	var otherRuntimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, f.Agents.Customs).Scan(&otherRuntimeID)
	running := dbfx.Task(t, f.Agents.Customs, testutil.Cols{"runtime_id": otherRuntimeID, "issue_id": docs, "status": "running", "started_at": time.Now()})
	// The close comes from a member, as the handler builds it for a user's status change.
	actor := testHandler.taskCancellationActor(ctx, "member", testUserID)

	taskIssue := func() (issue db.Issue) {
		var err error
		issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(docs))
		if err != nil {
			t.Fatalf("load task: %v", err)
		}
		return issue
	}
	runStatus := func(id string) string {
		var status string
		dbfx.QueryRow(t, `SELECT status FROM agent_task_queue WHERE id = $1`, id).Scan(&status)
		return status
	}

	// Still open: nothing is cancelled.
	testHandler.cancelQueuedRunsOfClosedFlowTask(ctx, taskIssue(), actor)
	if got := runStatus(queued); got != "queued" {
		t.Fatalf("queued run of an open task = %q, want queued", got)
	}

	// Closed: the queued run is cancelled and the running one is left alone.
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'done' WHERE id = $1`, parseUUID(docs)); err != nil {
		t.Fatalf("close task: %v", err)
	}
	testHandler.cancelQueuedRunsOfClosedFlowTask(ctx, taskIssue(), actor)
	if got := runStatus(queued); got != "cancelled" {
		t.Fatalf("queued run of a closed task = %q, want cancelled", got)
	}
	if got := runStatus(running); got != "running" {
		t.Fatalf("running run of a closed task = %q, want running", got)
	}
}
