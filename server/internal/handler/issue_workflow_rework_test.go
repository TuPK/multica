package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A workflow whose review step sends rejected work back to In Progress and hands
// Blocked to an agent (the SDLC lead) blocks the third return instead.
func reworkProject(t *testing.T, title, agentID string) string {
	t.Helper()
	projectID := deliveryProject(t, title, agentID)
	setColumnHandoff(t, projectID, "in_review", map[string]any{
		"handler": map[string]any{"type": "agent", "id": agentID}, "instructions": "Review it.", "back_status_key": "in_progress",
	}).Want(http.StatusOK)
	setColumnHandoff(t, projectID, "blocked", handToAgent(agentID, "Route the blocker.")).Want(http.StatusOK)
	return projectID
}

// reviewRun makes the agent's run on an issue that sits in review, as the review
// step's handler, and returns the run's id.
func reviewRun(t *testing.T, issueID, agentID string) string {
	t.Helper()
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	taskID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID, "issue_id": issueID, "status": "running",
		"started_at": testutil.Raw("now()"), "workflow_step": "in_review",
	})
	return taskID
}

func recordReturns(t *testing.T, issueID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		dbfx.Exec(t, `INSERT INTO activity_log (workspace_id, issue_id, actor_type, action, details)
			VALUES ($1, $2, 'agent', 'status_changed', '{"from":"in_review","to":"in_progress"}')`, testWorkspaceID, issueID)
	}
}

func TestThirdReturnFromReviewBlocksTheIssueForTheLead(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	projectID := reworkProject(t, "Rework third return project", agentID)
	issueID := dbfx.Issue(t, "returned three times", testutil.Cols{
		"project_id": projectID, "status": "in_review", "assignee_type": "agent", "assignee_id": agentID,
	})
	recordReturns(t, issueID, 2)
	taskID := reviewRun(t, issueID, agentID)

	agentUpdateIssue(t, issueID, agentID, taskID, map[string]any{"status": "in_progress"}).Want(http.StatusOK)

	if status, _, _ := issueStatusAndAssignee(t, issueID); status != "blocked" {
		t.Fatalf("status after the third return = %q, want blocked", status)
	}
	var content string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, issueID).Scan(&content)
	for _, want := range []string{"BLOCKED", "Blocker-class: rework-loop", "Blocked-from: in_progress", "3 times"} {
		if !strings.Contains(content, want) {
			t.Fatalf("rework report is missing %q:\n%s", want, content)
		}
	}
}

func TestSecondReturnFromReviewStillGoesBackToInProgress(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	projectID := reworkProject(t, "Rework second return project", agentID)
	issueID := dbfx.Issue(t, "returned twice", testutil.Cols{
		"project_id": projectID, "status": "in_review", "assignee_type": "agent", "assignee_id": agentID,
	})
	recordReturns(t, issueID, 1)
	taskID := reviewRun(t, issueID, agentID)

	agentUpdateIssue(t, issueID, agentID, taskID, map[string]any{"status": "in_progress"}).Want(http.StatusOK)

	if status, _, _ := issueStatusAndAssignee(t, issueID); status != "in_progress" {
		t.Fatalf("status after the second return = %q, want in_progress", status)
	}
}

// A block restarts the count: the lead's ruling resumes the issue and the next
// round of review gets its own two returns.
func TestReturnCountRestartsAfterABlock(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	projectID := reworkProject(t, "Rework restart project", agentID)
	issueID := dbfx.Issue(t, "returned after a block", testutil.Cols{
		"project_id": projectID, "status": "in_review", "assignee_type": "agent", "assignee_id": agentID,
	})
	recordReturns(t, issueID, 2)
	dbfx.Exec(t, `INSERT INTO activity_log (workspace_id, issue_id, actor_type, action, details, created_at)
		VALUES ($1, $2, 'agent', 'status_changed', '{"from":"in_progress","to":"blocked"}', now() + interval '1 second')`, testWorkspaceID, issueID)
	taskID := reviewRun(t, issueID, agentID)

	agentUpdateIssue(t, issueID, agentID, taskID, map[string]any{"status": "in_progress"}).Want(http.StatusOK)

	if status, _, _ := issueStatusAndAssignee(t, issueID); status != "in_progress" {
		t.Fatalf("status after a block and one return = %q, want in_progress", status)
	}
}

// A member who moves the issue is not the review step's agent; the rule leaves it alone.
func TestMemberReturnIsNeverTurnedIntoABlock(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	projectID := reworkProject(t, "Rework member project", agentID)
	issueID := dbfx.Issue(t, "member returns it", testutil.Cols{
		"project_id": projectID, "status": "in_review", "assignee_type": "agent", "assignee_id": agentID,
	})
	recordReturns(t, issueID, 5)

	updateIssueForTest(t, issueID, map[string]any{"status": "in_progress"}).Want(http.StatusOK)

	if status, _, _ := issueStatusAndAssignee(t, issueID); status != "in_progress" {
		t.Fatalf("a member's return became %q, want in_progress", status)
	}
}
