package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A run that fails for good, with nothing queued after it, leaves its issue
// blocked: the work cannot continue until someone fixes the cause. The issue
// must not roll back to todo, which says the issue is waiting for work that
// nothing will start.
func TestFailedRunWithoutRetryLeavesIssueBlocked(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	issueID := dbfx.Issue(t, "failed run blocks issue", testutil.Cols{
		"status": "in_progress", "assignee_type": "agent", "assignee_id": agentID,
	})
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	taskID := dbfx.Task(t, agentID, testutil.Cols{
		"issue_id": issueID, "runtime_id": runtimeID, "status": "running", "started_at": time.Now(),
	})
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'failed', failure_reason = 'agent_error', error = 'quota exhausted', completed_at = now() WHERE id = $1`, taskID)
	task, err := testHandler.Queries.GetAgentTask(context.Background(), util.MustParseUUID(taskID))
	if err != nil {
		t.Fatalf("load failed task: %v", err)
	}

	testHandler.TaskService.HandleFailedTasks(context.Background(), []db.AgentTaskQueue{task})
	if status, _, _ := issueStatusAndAssignee(t, issueID); status != "blocked" {
		t.Fatalf("issue status = %q after a run failed without retry, want blocked", status)
	}
}

// Moving an issue to todo from any status but backlog starts its assigned
// agent, as the status catalog promises. Without this a returned or reopened
// issue waits in todo for a run that is never queued.
func TestMovingToTodoStartsAssignedAgent(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	issueID := dbfx.Issue(t, "returned to todo", testutil.Cols{
		"status": "blocked", "assignee_type": "agent", "assignee_id": agentID,
	})

	updateIssueForTest(t, issueID, map[string]any{"status": "todo"}).Want(http.StatusOK)
	var queued int
	dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued'`, issueID, agentID).Scan(&queued)
	if queued != 1 {
		t.Fatalf("queued runs after moving to todo = %d, want one", queued)
	}
}
