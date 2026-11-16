package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
)

// Reproduction for harness-optimization-plan W11 (finding S7): an issue given to a squad that moves to
// in_review should get a run for the reviewer of that step. The three tests cover the ways an issue reaches
// in_review. Each one asserts the expected handoff. They are written before any fix (plan Q5): a failure here
// is the reproduction; the cause is in issue_workflow_entry.go, which this change does not touch.

// reproSquad returns a reviewer agent, a squad leader, the squad, and the runtime they share.
func reproSquad(t *testing.T) (reviewer, leader, squadID, runtimeID string) {
	t.Helper()
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	reviewer = seededReadyAgentID(t)
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, reviewer).Scan(&runtimeID)
	leader = dbfx.Agent(t, "repro leader "+workflowTestSuffix(), runtimeID)
	squadID = dbfx.Squad(t, "repro squad "+workflowTestSuffix(), leader)
	return reviewer, leader, squadID, runtimeID
}

// reproReviewerProject is a delivery project whose in_review step hands the issue to reviewer.
func reproReviewerProject(t *testing.T, reviewer string) string {
	t.Helper()
	projectID := deliveryProject(t, "repro in_review project "+workflowTestSuffix(), reviewer)
	setColumnHandoff(t, projectID, "in_review", map[string]any{
		"handler": map[string]any{"type": "agent", "id": reviewer}, "instructions": "Review the change.",
	}).Want(http.StatusOK)
	return projectID
}

// reproExpectReviewer fails unless the issue is now assigned to reviewer and has a run for it.
func reproExpectReviewer(t *testing.T, issueID, reviewer string) {
	t.Helper()
	status, assigneeType, assigneeID := issueStatusAndAssignee(t, issueID)
	tasks, err := testHandler.Queries.ListTasksByIssue(context.Background(), util.MustParseUUID(issueID))
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if assigneeID != reviewer || len(tasks) == 0 {
		t.Fatalf("status %q, assignee %s/%s, runs %d: want the in_review reviewer %s to be assigned and to get a run",
			status, assigneeType, assigneeID, len(tasks), reviewer)
	}
}

// A member moves a top-level request, given to a squad, into in_review.
func TestReproSquadRequestMovedToInReviewByMemberGetsReviewerRun(t *testing.T) {
	reviewer, _, squadID, _ := reproSquad(t)
	projectID := reproReviewerProject(t, reviewer)
	request := dbfx.Issue(t, "repro request in review", testutil.Cols{
		"project_id": projectID, "status": "in_progress", "assignee_type": "squad", "assignee_id": squadID,
	})
	updateIssueForTest(t, request, map[string]any{"status": "in_review"}).Want(http.StatusOK)
	reproExpectReviewer(t, request, reviewer)
}

// The squad leader's own run moves the request into in_review.
func TestReproSquadRequestMovedToInReviewByLeaderRunGetsReviewerRun(t *testing.T) {
	reviewer, leader, squadID, runtimeID := reproSquad(t)
	projectID := reproReviewerProject(t, reviewer)
	request := dbfx.Issue(t, "repro request moved by leader", testutil.Cols{
		"project_id": projectID, "status": "in_progress", "assignee_type": "squad", "assignee_id": squadID,
	})
	task := dbfx.Task(t, leader, testutil.Cols{
		"issue_id": request, "squad_id": squadID, "runtime_id": runtimeID, "status": "running", "started_at": time.Now(),
	})
	agentUpdateIssue(t, request, leader, task, map[string]any{"status": "in_review"}).Want(http.StatusOK)
	reproExpectReviewer(t, request, reviewer)
}

// A member moves an issue that is a piece of a request, given to a squad, into in_review.
func TestReproSquadChildMovedToInReviewByMemberGetsReviewerRun(t *testing.T) {
	reviewer, _, squadID, _ := reproSquad(t)
	projectID := reproReviewerProject(t, reviewer)
	request := dbfx.Issue(t, "repro parent request", testutil.Cols{"project_id": projectID, "status": "in_progress"})
	child := dbfx.Issue(t, "repro child in review", testutil.Cols{
		"project_id": projectID, "status": "in_progress", "assignee_type": "squad", "assignee_id": squadID, "parent_issue_id": request,
	})
	updateIssueForTest(t, child, map[string]any{"status": "in_review"}).Want(http.StatusOK)
	reproExpectReviewer(t, child, reviewer)
}
