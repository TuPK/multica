package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// An agent may not close a parent while its sub-issues are still open: the
// stage barrier that would wake the parent is silent once the parent is closed,
// so the open sub-issues would be left without anyone to advance them.
func TestAgentCannotCloseParentWithOpenSubIssues(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	parentID := dbfx.Issue(t, "parent with open sub-issues", testutil.Cols{
		"status": "in_progress", "assignee_type": "agent", "assignee_id": agentID,
	})
	childID := dbfx.Issue(t, "open sub-issue", testutil.Cols{"parent_issue_id": parentID, "status": "backlog"})
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	taskID := dbfx.Task(t, agentID, testutil.Cols{
		"issue_id": parentID, "runtime_id": runtimeID, "status": "running", "started_at": time.Now(),
	})

	body := agentUpdateIssue(t, parentID, agentID, taskID, map[string]any{"status": "done"}).Want(http.StatusConflict).Map()
	if body["code"] != "sub_issues_open" {
		t.Fatalf("refusal code = %v, want sub_issues_open: %v", body["code"], body)
	}
	if status, _, _ := issueStatusAndAssignee(t, parentID); status != "in_progress" {
		t.Fatalf("parent status = %q after the refused close, want in_progress", status)
	}

	// Once the sub-issue is closed the agent may close the parent.
	updateIssueForTest(t, childID, map[string]any{"status": "cancelled"}).Want(http.StatusOK)
	agentUpdateIssue(t, parentID, agentID, taskID, map[string]any{"status": "done"}).Want(http.StatusOK)
}

// A member decides for the team and may close a parent with open sub-issues.
func TestMemberMayCloseParentWithOpenSubIssues(t *testing.T) {
	seedTestCatalog(t)
	parentID := dbfx.Issue(t, "member closes parent", testutil.Cols{"status": "in_progress"})
	dbfx.Issue(t, "open sub-issue for member close", testutil.Cols{"parent_issue_id": parentID, "status": "backlog"})

	updateIssueForTest(t, parentID, map[string]any{"status": "done"}).Want(http.StatusOK)
}
