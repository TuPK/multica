package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"strconv"
)

// Auto-approvals of a restricted project's issues stay out of the policy history,
// the digest FYI and the weekly review event for a member with no project_member
// row (spec 7.1 rule 5). The owner still sees them in the history and the digest.

type policyVisibilityFixture struct {
	digestVisibilityFixture
	policyID string
}

func newPolicyVisibilityFixture(t *testing.T) policyVisibilityFixture {
	t.Helper()
	f := policyVisibilityFixture{digestVisibilityFixture: newDigestVisibilityFixture(t)}
	f.policyID = dbfx.Insert(t, "approval_policy", testutil.Cols{
		"workspace_id": f.WorkspaceID, "name": "Visibility policy", "kind": "decision",
		"match": "{}", "created_by": testUserID,
	})
	return f
}

// autoApproved inserts an auto-approved proposal on a new issue of the project.
// A reversed one carries the reversal note too.
func (f policyVisibilityFixture) autoApproved(t *testing.T, title, project string, reversed bool) (proposalID string, number int) {
	t.Helper()
	issueCols := testutil.Cols{"workspace_id": f.WorkspaceID}
	if project != "" {
		issueCols["project_id"] = project
	}
	issue := dbfx.Issue(t, title, issueCols)
	cols := testutil.Cols{
		"workspace_id": f.WorkspaceID, "squad_id": f.squadID, "issue_id": issue,
		"created_by_type": "agent", "created_by_id": f.AgentID,
		"state": "approved", "decided_by_policy_id": f.policyID,
		"decided_at": testutil.Raw("now() - interval '1 hour'"),
	}
	if reversed {
		cols["reversed_at"] = testutil.Raw("now() - interval '30 minutes'")
		cols["reversal_note"] = "reversed on review"
	}
	return dbfx.Insert(t, "proposal", cols), f.issueNumber(t, issue)
}

func (f policyVisibilityFixture) history(t *testing.T, userID string) (string, ListPolicyAutoApprovalsResponse) {
	t.Helper()
	req := requestAs(userID, http.MethodGet, "/api/approval-policies/"+f.policyID+"/auto-approvals", nil)
	req.Header.Set("X-Workspace-ID", f.WorkspaceID)
	var out ListPolicyAutoApprovalsResponse
	res := testutil.Call(t, testHandler.ListPolicyAutoApprovals, testutil.WithURLParams(req, "id", f.policyID)).Want(http.StatusOK)
	res.JSON(&out)
	return res.Text(), out
}

func TestApprovalPolicyVisibility_HistoryAndDigestFYI(t *testing.T) {
	f := newPolicyVisibilityFixture(t)
	_, hiddenNum := f.autoApproved(t, "HIDDEN-AUTOAPPROVED-TITLE", f.hiddenProj, false)
	_, hiddenRevNum := f.autoApproved(t, "HIDDEN-REVERSED-TITLE", f.hiddenProj, true)
	f.autoApproved(t, "OPEN-AUTOAPPROVED-TITLE", f.openProj, false)

	raw, hist := f.history(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-AUTOAPPROVED-TITLE", "HIDDEN-REVERSED-TITLE",
		"DIG-"+strconv.Itoa(hiddenNum)+`"`, "DIG-"+strconv.Itoa(hiddenRevNum)+`"`)
	requireShown(t, "member", raw, "OPEN-AUTOAPPROVED-TITLE")
	if len(hist.AutoApprovals) != 1 || hist.Flagged {
		t.Errorf("member history = %d rows, flagged=%v; want 1 row, not flagged", len(hist.AutoApprovals), hist.Flagged)
	}
	raw, hist = f.history(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-AUTOAPPROVED-TITLE", "HIDDEN-REVERSED-TITLE", "OPEN-AUTOAPPROVED-TITLE")
	if len(hist.AutoApprovals) != 3 || !hist.Flagged {
		t.Errorf("owner history = %d rows, flagged=%v; want 3 rows, flagged", len(hist.AutoApprovals), hist.Flagged)
	}

	// The digest FYI lists recent auto-approvals.
	raw, _ = f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-AUTOAPPROVED-TITLE", "HIDDEN-REVERSED-TITLE")
	requireShown(t, "member", raw, "OPEN-AUTOAPPROVED-TITLE")
	raw, _ = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-AUTOAPPROVED-TITLE", "OPEN-AUTOAPPROVED-TITLE")
}

func TestApprovalPolicyVisibility_WeeklyReviewEvent(t *testing.T) {
	f := newPolicyVisibilityFixture(t)
	hiddenID, _ := f.autoApproved(t, "HIDDEN-REVERSED-TITLE", f.hiddenProj, true)
	// The review has no viewer, so it uses the zero scope: only issues without a
	// project remain. An issue of a workspace-access project is left out too.
	openID, _ := f.autoApproved(t, "OPEN-REVERSED-TITLE", "", true)
	f.autoApproved(t, "OPEN-PROJECT-REVERSED-TITLE", f.openProj, true)

	svc := &service.ApprovalPolicyService{Queries: testHandler.Queries}
	n, err := svc.RecordWeeklyPolicyReview(t.Context(), util.MustParseUUID(f.WorkspaceID), time.Now())
	if err != nil {
		t.Fatalf("RecordWeeklyPolicyReview: %v", err)
	}
	if n != 1 {
		t.Errorf("review lists %d reversals, want 1 (the hidden one is left out)", n)
	}
	var summary, details string
	dbfx.QueryRow(t, `SELECT summary, details::text FROM governance_event
		WHERE workspace_id = $1 AND kind = 'review_published'`, f.WorkspaceID).Scan(&summary, &details)
	event := summary + details
	if strings.Contains(event, "HIDDEN-REVERSED-TITLE") || strings.Contains(event, hiddenID) {
		t.Errorf("governance event leaks the hidden issue: %s", event)
	}
	if !strings.Contains(event, "OPEN-REVERSED-TITLE") || !strings.Contains(event, openID) {
		t.Errorf("governance event misses the visible reversal: %s", event)
	}
}
