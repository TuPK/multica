package handler

// A squad filer resubmits a proposal by id (spec 7.1 rules 3, 5). Proposal
// issues are filed without a project; once an admin moves one into a
// restricted project, a squad member who does not see that project gets the
// answer a missing proposal gets and changes nothing.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestRestrictedProposalResubmitAnswersMissing: the outsider is a member of
// the proposal's squad, so only visibility can refuse the resubmission.
func TestRestrictedProposalResubmitAnswersMissing(t *testing.T) {
	fx := newProposalTestFixture(t)
	f := newRestrictedFixture(t)
	dbfx.SquadMember(t, fx.SquadID, "member", f.outsiderID)
	dbfx.SquadMember(t, fx.SquadID, "member", f.granteeID)

	p := fx.fileProposal(t, "hidden resubmit", nil)
	reviseProposal(t, p.ID, "Narrow the ask.")
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, p.IssueID, f.restrictedProjectID)

	resubmitAs := func(userID, proposalID string) *httptest.ResponseRecorder {
		body := proposalBody(fx.SquadID, fx.title("hidden resubmit"), map[string]any{"description": "Outsider rewrite."})
		delete(body, "squad_id")
		req := withURLParam(requestAs(userID, "POST", "/api/proposals/"+proposalID+"/revision", body), "id", proposalID)
		return serveWorkflowRoute(testHandler.ReviseProposal, req)
	}

	missingID := uuid.NewString()
	answersMissing(t, "ReviseProposal", resubmitAs(f.outsiderID, p.ID), resubmitAs(f.outsiderID, missingID), p.ID, missingID)
	if got := proposalState(t, p.ID); got != "revising" {
		t.Fatalf("the outsider's resubmission changed the proposal: state = %q, want revising", got)
	}
	var description string
	dbfx.QueryRow(t, `SELECT description FROM issue WHERE id = $1`, p.IssueID).Scan(&description)
	if want := "Outsider rewrite."; strings.Contains(description, want) {
		t.Fatalf("the outsider's text reached the issue description: %q", description)
	}

	// The grantee sees the project, so the same call goes through.
	if w := resubmitAs(f.granteeID, p.ID); w.Code != http.StatusOK {
		t.Fatalf("ReviseProposal as the grantee: want 200, got %d %s", w.Code, w.Body.String())
	}
}
