package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Closing a flow task (R1 and R2 of docs/engineering/kitviet-workflow-spec.md,
// section 4.2). The rules hold for every write path that changes a status and
// for every caller.

func TestFlowTaskClose_GateWithoutOutcome422(t *testing.T) {
	fx := newFlowFixture(t)

	body := wantFlowRejection(t, closeIssue(t, fx.Gate, nil), http.StatusUnprocessableEntity, "flow_outcome_required")
	allowed, _ := body["allowed_outcomes"].([]any)
	if len(allowed) != 2 || allowed[0] != "approved" || allowed[1] != "rejected" {
		t.Fatalf("allowed_outcomes = %v, want [approved rejected]", allowed)
	}
	if body["task"] != "check" {
		t.Fatalf("task = %v, want check", body["task"])
	}
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "in_progress" {
		t.Fatalf("status = %q, a refused close must not apply", status)
	}
	if got := issueOutcome(t, fx.Gate); got != "" {
		t.Fatalf("outcome = %q, want none", got)
	}

	var closed IssueResponse
	closeIssue(t, fx.Gate, map[string]any{"outcome": "approved"}).Want(http.StatusOK).JSON(&closed)
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "done" {
		t.Fatalf("status = %q, want done", status)
	}
	if got := issueOutcome(t, fx.Gate); got != "approved" {
		t.Fatalf("stored outcome = %q, want approved", got)
	}
	if closed.Outcome == nil || *closed.Outcome != "approved" {
		t.Fatalf("response outcome = %v, want approved", closed.Outcome)
	}
}

func TestFlowTaskClose_DoneWithBlockedOutcome422(t *testing.T) {
	fx := newFlowFixture(t)

	// A gate closes only with a verdict.
	wantFlowRejection(t, closeIssue(t, fx.Gate, map[string]any{"outcome": "blocked"}), http.StatusUnprocessableEntity, "flow_outcome_invalid")
	wantFlowRejection(t, closeIssue(t, fx.Gate, map[string]any{"outcome": "done"}), http.StatusUnprocessableEntity, "flow_outcome_invalid")
	// A task that cannot go on is blocked, not done, and its outcome says so.
	wantFlowRejection(t, closeIssue(t, fx.Work, map[string]any{"outcome": "blocked"}), http.StatusUnprocessableEntity, "flow_outcome_invalid")
	wantFlowRejection(t, updateIssueForTest(t, fx.Work, map[string]any{"status": "blocked", "outcome": "approved"}),
		http.StatusUnprocessableEntity, "flow_outcome_invalid")
	if status, _, _ := issueStatusAndAssignee(t, fx.Work); status != "in_progress" {
		t.Fatalf("status = %q, a refused write must not apply", status)
	}

	updateIssueForTest(t, fx.Work, map[string]any{"status": "blocked", "outcome": "blocked"}).Want(http.StatusOK)
	if status, _, _ := issueStatusAndAssignee(t, fx.Work); status != "blocked" {
		t.Fatalf("status = %q, want blocked", status)
	}
	if got := issueOutcome(t, fx.Work); got != "blocked" {
		t.Fatalf("stored outcome = %q, want blocked", got)
	}
}

func TestFlowTaskClose_AttachmentRequired422(t *testing.T) {
	fx := newFlowFixture(t)

	body := wantFlowRejection(t, closeIssue(t, fx.Work, nil), http.StatusUnprocessableEntity, "flow_attachment_required")
	if body["task"] != "draft" {
		t.Fatalf("task = %v, want draft", body["task"])
	}
	if status, _, _ := issueStatusAndAssignee(t, fx.Work); status != "in_progress" {
		t.Fatalf("status = %q, a refused close must not apply", status)
	}

	attachAs(t, fx.Work, fx.WorkAgent)
	closeIssue(t, fx.Work, nil).Want(http.StatusOK)
	if status, _, _ := issueStatusAndAssignee(t, fx.Work); status != "done" {
		t.Fatalf("status = %q, want done", status)
	}
}

// A file linked to a comment keeps the issue's id, so it counts; a file another
// agent or another member uploaded does not (one the closing member uploaded does:
// see TestFlowGuard_ManagerOnBehalfStillNeedsTheFiles).
func TestFlowTaskClose_AttachmentOnCommentCounts(t *testing.T) {
	fx := newFlowFixture(t)

	other := goldenLeadAgent(t, fx.WorkAgent)
	attachAs(t, fx.Work, other)
	otherMember, _ := addWorkflowTestMember(t, "flow-other-uploader")
	dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": fx.Work,
		"uploader_type": "member", "uploader_id": otherMember,
		"filename": "mine.md", "url": "https://files.example.test/mine.md",
		"content_type": "text/markdown", "size_bytes": 4,
	})
	wantFlowRejection(t, closeIssue(t, fx.Work, nil), http.StatusUnprocessableEntity, "flow_attachment_required")

	comment := dbfx.Comment(t, fx.Work, "Here is the draft.", testutil.Cols{"author_type": "agent", "author_id": fx.WorkAgent})
	dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": fx.Work, "comment_id": comment,
		"uploader_type": "agent", "uploader_id": fx.WorkAgent,
		"filename": "draft.md", "url": "https://files.example.test/draft2.md",
		"content_type": "text/markdown", "size_bytes": 8,
	})
	closeIssue(t, fx.Work, nil).Want(http.StatusOK)
}

func TestFlowTaskClose_BatchPathCovered(t *testing.T) {
	fx := newFlowFixture(t)

	batch := func(updates map[string]any, ids ...string) *testutil.Response {
		return testutil.Call(t, testHandler.BatchUpdateIssues, newRequest("POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID, map[string]any{
			"issue_ids": ids, "updates": updates,
		}))
	}

	wantFlowRejection(t, batch(map[string]any{"status": "done"}, fx.Gate), http.StatusUnprocessableEntity, "flow_outcome_required")
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "in_progress" {
		t.Fatalf("the batch closed a gate with no verdict: status %q", status)
	}
	// One refused item refuses the batch: nothing is written.
	plain := dbfx.Issue(t, "plain issue in the batch "+workflowTestSuffix())
	wantFlowRejection(t, batch(map[string]any{"status": "done"}, plain, fx.Gate), http.StatusUnprocessableEntity, "flow_outcome_required")
	if status, _, _ := issueStatusAndAssignee(t, plain); status != "todo" {
		t.Fatalf("the batch wrote the plain issue (status %q) although another item was refused", status)
	}

	batch(map[string]any{"status": "done", "outcome": "rejected"}, fx.Gate).Want(http.StatusOK)
	if got := issueOutcome(t, fx.Gate); got != "rejected" {
		t.Fatalf("stored outcome = %q, want rejected: the batch must pass the outcome of the update", got)
	}
}

func TestOutcomeStoredAndReturned(t *testing.T) {
	seedTestCatalog(t)
	issueID := dbfx.Issue(t, "outcome stored "+workflowTestSuffix(), testutil.Cols{"status": "in_review"})

	var got IssueResponse
	updateIssueForTest(t, issueID, map[string]any{"status": "done", "outcome": "approved"}).Want(http.StatusOK).JSON(&got)
	if got.Outcome == nil || *got.Outcome != "approved" {
		t.Fatalf("response outcome = %v, want approved", got.Outcome)
	}
	if stored := issueOutcome(t, issueID); stored != "approved" {
		t.Fatalf("stored outcome = %q, want approved", stored)
	}
	var read IssueResponse
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest("GET", "/api/issues/"+issueID, nil), "id", issueID)).Want(http.StatusOK).JSON(&read)
	if read.Outcome == nil || *read.Outcome != "approved" {
		t.Fatalf("read outcome = %v, want approved", read.Outcome)
	}

	// Reopening without a verdict clears it: the outcome belongs to the move that set it.
	var reopened IssueResponse // a fresh value: decoding into `got` would keep its old pointer
	updateIssueForTest(t, issueID, map[string]any{"status": "in_progress"}).Want(http.StatusOK).JSON(&reopened)
	if reopened.Outcome != nil || issueOutcome(t, issueID) != "" {
		t.Fatalf("outcome after reopening = %v / %q, want none", reopened.Outcome, issueOutcome(t, issueID))
	}
	// An outcome without a status change is not stored.
	updateIssueForTest(t, issueID, map[string]any{"title": "renamed " + workflowTestSuffix(), "outcome": "rejected"}).Want(http.StatusOK)
	if stored := issueOutcome(t, issueID); stored != "" {
		t.Fatalf("an outcome sent with no status change was stored: %q", stored)
	}
}

func TestPlainIssueUnaffected(t *testing.T) {
	seedTestCatalog(t)
	issueID := dbfx.Issue(t, "plain close "+workflowTestSuffix(), testutil.Cols{"status": "in_progress"})

	closeIssue(t, issueID, nil).Want(http.StatusOK)
	if status, _, _ := issueStatusAndAssignee(t, issueID); status != "done" {
		t.Fatalf("status = %q, want done", status)
	}
	if got := issueOutcome(t, issueID); got != "" {
		t.Fatalf("outcome = %q, want none", got)
	}

	// A sub-issue that is not a flow task closes as before, even under a flow request.
	fx := newFlowFixture(t)
	child := dbfx.Issue(t, "plain child "+workflowTestSuffix(), testutil.Cols{"status": "in_progress", "parent_issue_id": fx.Request})
	closeIssue(t, child, nil).Want(http.StatusOK)
}
