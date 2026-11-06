package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func storedRunStartStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	dbfx.QueryRow(t, `SELECT run_start_status FROM issue WHERE id = $1`, id).Scan(&status)
	return status
}

func TestPromoteQueuedIssues_StartsInTheIssuesStartStatus(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	plain := queueIssue(t, "queue default start")
	review := queueIssue(t, "queue review start")
	dbfx.Exec(t, `UPDATE issue SET run_start_status = 'in_review' WHERE id = $1`, review)

	if n := promoteQueue(t); n != 2 {
		t.Fatalf("moved %d issues, want 2", n)
	}
	if got := queuedIssueStatus(t, plain); got != "todo" {
		t.Fatalf("default start = %q, want todo", got)
	}
	if got := queuedIssueStatus(t, review); got != "in_review" {
		t.Fatalf("start status = %q, want in_review", got)
	}
}

func TestSetIssueRunQueue_StoresAndValidatesStartStatus(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	id := dbfx.Issue(t, "start status endpoint", testutil.Cols{"status": "backlog"})

	for _, status := range []string{"backlog", "done", "cancelled"} {
		if w := putRunQueue(t, id, `{"queued":true,"start_status":"`+status+`"}`); w.Code != http.StatusBadRequest {
			t.Fatalf("start_status %q: status = %d, want 400", status, w.Code)
		}
	}
	if w := putRunQueue(t, id, `{"queued":true,"start_status":"in_review"}`); w.Code != http.StatusOK {
		t.Fatalf("set: status = %d: %s", w.Code, w.Body.String())
	}
	if got := storedRunStartStatus(t, id); got != "in_review" {
		t.Fatalf("stored start status = %q, want in_review", got)
	}
	if w := putRunQueue(t, id, `{"queued":true}`); w.Code != http.StatusOK {
		t.Fatalf("set without status: status = %d", w.Code)
	}
	if got := storedRunStartStatus(t, id); got != "todo" {
		t.Fatalf("stored start status = %q, want the default todo", got)
	}
}

func TestSetIssueRunQueue_RefusesAStartStatusTheProjectWorkflowLacks(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	wf := createTestWorkflow(t, "Start status "+workflowTestSuffix(), "in_review")
	projectID := createWorkflowTestProject(t, "Start status project")
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": wf.ID}).Want(http.StatusOK)
	id := dbfx.Issue(t, "start status workflow", testutil.Cols{"project_id": projectID, "status": "backlog"})

	if w := putRunQueue(t, id, `{"queued":true,"start_status":"architecture"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unlisted status: status = %d, want 400", w.Code)
	}
	if w := putRunQueue(t, id, `{"queued":true,"start_status":"in_review"}`); w.Code != http.StatusOK {
		t.Fatalf("listed status: status = %d: %s", w.Code, w.Body.String())
	}
}

// A request a squad leader is working is not a step of the workflow when a
// member moves it: that does not hand it to the step's handler. The same move on
// the leader's child issue, or on an issue nobody leads, does, and so does the
// leader's own run moving the request (TestSquadLeaderMoveHandsRequestToStepHandler).
func TestWorkflowDoesNotHandOffASquadRequest(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	handler := seededReadyAgentID(t)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, handler).Scan(&runtimeID)
	leader := dbfx.Agent(t, "request leader "+workflowTestSuffix(), runtimeID)
	dbfx.Squad(t, "request squad "+workflowTestSuffix(), leader)
	projectID := deliveryProject(t, "Squad request project", handler)

	request := dbfx.Issue(t, "the request", testutil.Cols{
		"project_id": projectID, "status": "backlog", "assignee_type": "agent", "assignee_id": leader,
	})
	updateIssueForTest(t, request, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	if status, assigneeType, assigneeID := issueStatusAndAssignee(t, request); status != "in_progress" || assigneeType != "agent" || assigneeID != leader {
		t.Fatalf("request = (%q, %q, %q), want in_progress still assigned to the leader", status, assigneeType, assigneeID)
	}

	child := dbfx.Issue(t, "a piece of the request", testutil.Cols{
		"project_id": projectID, "status": "backlog", "assignee_type": "agent", "assignee_id": leader, "parent_issue_id": request,
	})
	updateIssueForTest(t, child, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	if _, _, assigneeID := issueStatusAndAssignee(t, child); assigneeID != handler {
		t.Fatalf("child assignee = %q, want the step's handler %q", assigneeID, handler)
	}

	other := dbfx.Issue(t, "not a request", testutil.Cols{
		"project_id": projectID, "status": "backlog", "assignee_type": "agent", "assignee_id": runtimeIDAgent(t, runtimeID),
	})
	updateIssueForTest(t, other, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	if _, _, assigneeID := issueStatusAndAssignee(t, other); assigneeID != handler {
		t.Fatalf("other assignee = %q, want the step's handler %q", assigneeID, handler)
	}
}

// The leader's own run moves its request into a step another agent handles: the
// step's handler takes the request, so the work does not stop at the leader.
func TestSquadLeaderMoveHandsRequestToStepHandler(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	handler := seededReadyAgentID(t)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, handler).Scan(&runtimeID)
	leader := dbfx.Agent(t, "handoff leader "+workflowTestSuffix(), runtimeID)
	squadID := dbfx.Squad(t, "handoff squad "+workflowTestSuffix(), leader)
	// A leader may assign only to agents in its squad, and the step's handler is one.
	dbfx.Exec(t, `INSERT INTO squad_member (squad_id, member_type, member_id) VALUES ($1, 'agent', $2)`, squadID, handler)
	projectID := deliveryProject(t, "Squad leader handoff project", handler)

	request := dbfx.Issue(t, "a request the leader sent back", testutil.Cols{
		"project_id": projectID, "status": "blocked", "assignee_type": "agent", "assignee_id": leader,
	})
	task := dbfx.Task(t, leader, testutil.Cols{
		"issue_id": request, "runtime_id": runtimeID, "status": "running", "started_at": time.Now(),
	})

	agentUpdateIssue(t, request, leader, task, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	if status, assigneeType, assigneeID := issueStatusAndAssignee(t, request); status != "in_progress" || assigneeType != "agent" || assigneeID != handler {
		t.Fatalf("request = (%q, %q, %q), want in_progress handed to the step's handler %q", status, assigneeType, assigneeID, handler)
	}
}

// runtimeIDAgent makes an agent that leads no squad.
func runtimeIDAgent(t *testing.T, runtimeID string) string {
	t.Helper()
	return dbfx.Agent(t, "plain agent "+workflowTestSuffix(), runtimeID)
}
