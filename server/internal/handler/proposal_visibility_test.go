package handler

// A proposal read must not reveal the title, status or number of a backing
// issue the viewer cannot see (spec 7.1 rule 5). Proposal issues are filed
// without a project; an admin then moves one into a restricted project.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func listProposalsAs(t *testing.T, userID string) []ProposalResponse {
	t.Helper()
	var out []ProposalResponse
	req := requestAs(userID, "GET", "/api/proposals", nil)
	testutil.Call(t, testHandler.ListProposals, req).Want(http.StatusOK).JSON(&out)
	return out
}

func getProposalAs(userID, proposalID string) *httptest.ResponseRecorder {
	req := withURLParam(requestAs(userID, "GET", "/api/proposals/"+proposalID, nil), "id", proposalID)
	return serveWorkflowRoute(testHandler.GetProposal, req)
}

func containsProposal(list []ProposalResponse, id string) bool {
	for _, p := range list {
		if p.ID == id {
			return true
		}
	}
	return false
}

func TestProposalListOmitsHiddenBackingIssue(t *testing.T) {
	fx := newProposalTestFixture(t)
	f := newRestrictedFixture(t)
	hidden := fx.fileProposal(t, "hidden list", nil)
	visible := fx.fileProposal(t, "visible list", nil)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, hidden.IssueID, f.restrictedProjectID)

	for who, userID := range map[string]string{"outsider": f.outsiderID} {
		list := listProposalsAs(t, userID)
		if containsProposal(list, hidden.ID) {
			t.Errorf("ListProposals as %s lists the proposal of a hidden issue", who)
		}
		if !containsProposal(list, visible.ID) {
			t.Errorf("ListProposals as %s omits the proposal of a visible issue", who)
		}
	}
	for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if list := listProposalsAs(t, userID); !containsProposal(list, hidden.ID) || !containsProposal(list, visible.ID) {
			t.Errorf("ListProposals as %s must list both proposals", who)
		}
	}
}

func TestProposalGetHiddenBackingIssueAnswersMissing(t *testing.T) {
	fx := newProposalTestFixture(t)
	f := newRestrictedFixture(t)
	p := fx.fileProposal(t, "hidden get", nil)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, p.IssueID, f.restrictedProjectID)

	missingID := uuid.NewString()
	answersMissing(t, "GetProposal", getProposalAs(f.outsiderID, p.ID), getProposalAs(f.outsiderID, missingID), p.ID, missingID)

	for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		w := getProposalAs(userID, p.ID)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), p.IssueID) {
			t.Errorf("GetProposal as %s: want 200 with the proposal, got %d %s", who, w.Code, w.Body.String())
		}
	}
}
