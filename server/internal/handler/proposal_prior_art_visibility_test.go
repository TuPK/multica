package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// related[] on a proposal read names only proposals whose backing issue the
// viewer sees (spec 7.1 rule 5). A member with no project_member row must not
// learn the title of a proposal on a restricted project's issue.

func TestProposalPriorArtVisibility_RelatedOmitsHiddenProposals(t *testing.T) {
	fx := newProposalTestFixture(t)
	projectID := dbfx.Project(t, fx.title("restricted project"), testutil.Cols{"workspace_id": testWorkspaceID, "visibility": "private"})
	ownerJoinsProject(t, testWorkspaceID, projectID)
	memberID := dbfx.User(t, "Prior art member", testutil.UniqueEmail("prior-art-member"))
	dbfx.Member(t, testWorkspaceID, memberID, "member")

	hidden := fx.fileProposal(t, "pilot usage pricing", nil)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, hidden.IssueID, projectID)
	visibleEarlier := fx.fileProposal(t, "usage pricing pilot plan", nil)
	subject := fx.fileProposal(t, "usage pricing pilot", nil)

	list := func(userID string) ProposalResponse {
		t.Helper()
		var out []ProposalResponse
		req := requestAs(userID, "GET", "/api/proposals", nil)
		req.Header.Set("X-Workspace-ID", testWorkspaceID)
		testutil.Call(t, testHandler.ListProposals, req).Want(http.StatusOK).JSON(&out)
		for _, p := range out {
			if p.ID == subject.ID {
				return p
			}
		}
		t.Fatalf("proposal %s missing from the list of user %s", subject.ID, userID)
		return ProposalResponse{}
	}
	get := func(userID string) ProposalResponse {
		t.Helper()
		req := requestAs(userID, "GET", "/api/proposals/"+subject.ID, nil)
		req.Header.Set("X-Workspace-ID", testWorkspaceID)
		var out ProposalResponse
		testutil.Call(t, testHandler.GetProposal, withURLParam(req, "id", subject.ID)).Want(http.StatusOK).JSON(&out)
		return out
	}

	for name, read := range map[string]func(string) ProposalResponse{"list": list, "get": get} {
		m := read(memberID)
		if findRelated(m.Related, hidden.ID) != nil {
			t.Errorf("%s: member sees the proposal on the restricted project in related[]: %+v", name, m.Related)
		}
		if findRelated(m.Related, visibleEarlier.ID) == nil {
			t.Errorf("%s: member lost the visible related proposal: %+v", name, m.Related)
		}
		o := read(testUserID)
		if findRelated(o.Related, hidden.ID) == nil {
			t.Errorf("%s: owner must still see the hidden proposal in related[]: %+v", name, o.Related)
		}
	}
}
