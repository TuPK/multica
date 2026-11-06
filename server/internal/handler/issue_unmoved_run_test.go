package handler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// unmovedRunFixture is a delivery-workflow issue in progress for an agent, with a
// completed run that started at `started`. The run started in runStep.
func unmovedRunFixture(t *testing.T, name, runStep string) (issue, agent string, run *db.AgentTaskQueue) {
	t.Helper()
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	agent = seededReadyAgentID(t)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agent).Scan(&runtimeID)
	projectID := deliveryProject(t, name+" "+workflowTestSuffix(), agent)
	started := time.Now()
	issue = dbfx.Issue(t, name, testutil.Cols{
		"project_id": projectID, "status": "in_progress", "assignee_type": "agent", "assignee_id": agent,
	})
	dbfx.Task(t, agent, testutil.Cols{
		"issue_id": issue, "runtime_id": runtimeID, "status": "completed", "started_at": started,
	})
	run = &db.AgentTaskQueue{
		IssueID:      parseUUID(issue),
		AgentID:      parseUUID(agent),
		StartedAt:    pgtype.Timestamptz{Time: started, Valid: true},
		WorkflowStep: pgtype.Text{String: runStep, Valid: true},
	}
	return issue, agent, run
}

func issueTaskRows(t *testing.T, issue string) int {
	t.Helper()
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, issue).Scan(&n)
	return n
}

// A run that ended in the step it started in left the issue unmoved: the assignee is
// queued again with the brief to change approach.
func TestUnmovedWorkflowRunRequeuesAssignee(t *testing.T) {
	issue, _, run := unmovedRunFixture(t, "story the run left in progress", "in_progress")
	before := issueTaskRows(t, issue)
	testHandler.reconcileUnmovedWorkflowRun(context.Background(), run)
	if after := issueTaskRows(t, issue); after != before+1 {
		t.Fatalf("task rows = %d, want %d: an unmoved run must queue the assignee once more", after, before+1)
	}
}

// A run that started in another step moved the issue, so nothing is queued.
func TestMovedWorkflowRunIsNotRequeued(t *testing.T) {
	issue, _, run := unmovedRunFixture(t, "story the run moved", "todo")
	before := issueTaskRows(t, issue)
	testHandler.reconcileUnmovedWorkflowRun(context.Background(), run)
	if after := issueTaskRows(t, issue); after != before {
		t.Fatalf("task rows = %d, want %d: a run that moved the issue must not be re-run", after, before)
	}
}

// An issue whose current step has no agent handler may stay in progress on purpose,
// so an unmoved run on it is not re-run.
func TestUnmovedRunWithoutHandlerStepIsNotRequeued(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	agent := seededReadyAgentID(t)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agent).Scan(&runtimeID)
	started := time.Now()
	issue := dbfx.Issue(t, "plain issue left in progress", testutil.Cols{
		"status": "in_progress", "assignee_type": "agent", "assignee_id": agent,
	})
	run := &db.AgentTaskQueue{IssueID: parseUUID(issue), AgentID: parseUUID(agent), StartedAt: pgtype.Timestamptz{Time: started, Valid: true}}
	before := issueTaskRows(t, issue)
	testHandler.reconcileUnmovedWorkflowRun(context.Background(), run)
	if after := issueTaskRows(t, issue); after != before {
		t.Fatalf("task rows = %d, want %d: an issue with no handler step must not be re-run", after, before)
	}
}
