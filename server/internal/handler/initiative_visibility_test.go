package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// ListInitiatives and GetInitiative answer a member who has no project_member
// row as if what sits in a restricted project were missing (spec 7.1 rule 5):
// a gate whose backing issue is hidden is omitted, a hidden build project is
// null, and an initiative whose own issue is hidden is absent from the list
// and answers 404 by id.

func (fx iniWorkspace) listAs(t *testing.T, userID string) []InitiativeResponse {
	t.Helper()
	var out struct {
		Initiatives []InitiativeResponse `json:"initiatives"`
	}
	testutil.Call(t, testHandler.ListInitiatives, fx.as(newRequest("GET", "/api/initiatives", nil), userID)).
		Want(http.StatusOK).JSON(&out)
	return out.Initiatives
}

func (fx iniWorkspace) detailAs(t *testing.T, userID, id string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.GetInitiative,
		withURLParam(fx.as(newRequest("GET", "/api/initiatives/"+id, nil), userID), "id", id))
}

// hiddenGate files a pending stage gate for the initiative whose backing issue
// sits in a restricted project and carries a recognizable title.
func (fx iniWorkspace) hiddenGate(t *testing.T, initiativeID, title string) (proposalID string) {
	t.Helper()
	restricted := dbfx.Project(t, "Restricted gate "+uuid.NewString()[:8], testutil.Cols{"workspace_id": fx.ID, "visibility": "private"})
	ownerJoinsProject(t, fx.ID, restricted)
	issueID := dbfx.Issue(t, title, testutil.Cols{"workspace_id": fx.ID, "project_id": restricted})
	return dbfx.Insert(t, "proposal", testutil.Cols{
		"workspace_id":    fx.ID,
		"squad_id":        fx.Product,
		"issue_id":        issueID,
		"kind":            "stage_gate",
		"state":           "pending",
		"payload":         testutil.Raw(`'{"initiative_id":"` + initiativeID + `"}'::jsonb`),
		"created_by_type": "agent",
		"created_by_id":   fx.Leader,
	})
}

func TestInitiativeVisibility_HiddenGateIssueIsOmitted(t *testing.T) {
	fx := newRoomedIniWorkspace(t)
	ini := fx.createInitiative(t, "Gate visibility initiative")
	const hiddenTitle = "HIDDEN-GATE-ISSUE-TITLE"
	gateID := fx.hiddenGate(t, ini.ID, hiddenTitle)

	// The admin sees the gate, its title and the pending id.
	admin := fx.detail(t, ini.ID)
	if admin.Initiative.PendingGateProposalID == nil || *admin.Initiative.PendingGateProposalID != gateID {
		t.Fatalf("admin should see the pending gate %s, got %v", gateID, admin.Initiative.PendingGateProposalID)
	}
	if len(admin.Gates) != 1 || admin.Gates[0].Title != hiddenTitle {
		t.Fatalf("admin should see the gate with its title, got %+v", admin.Gates)
	}

	resp := fx.detailAs(t, fx.MemberID, ini.ID).Want(http.StatusOK)
	body := resp.Text()
	for _, leak := range []string{hiddenTitle, gateID} {
		if strings.Contains(body, leak) {
			t.Errorf("detail for a member without a grant leaks %q: %s", leak, body)
		}
	}
	var member InitiativeDetailResponse
	resp.JSON(&member)
	if len(member.Gates) != 0 || member.Initiative.PendingGateProposalID != nil {
		t.Errorf("hidden gate should be omitted, got gates=%d pending=%v", len(member.Gates), member.Initiative.PendingGateProposalID)
	}

	for _, v := range fx.listAs(t, fx.MemberID) {
		if v.ID == ini.ID && v.PendingGateProposalID != nil {
			t.Errorf("list for a member without a grant leaks the pending gate id %s", *v.PendingGateProposalID)
		}
	}
}

func TestInitiativeVisibility_HiddenBuildProjectIsNull(t *testing.T) {
	fx := newRoomedIniWorkspace(t)
	ini := fx.createInitiative(t, "Build project visibility initiative")
	restricted := dbfx.Project(t, "Restricted build "+uuid.NewString()[:8], testutil.Cols{"workspace_id": fx.ID, "visibility": "private"})
	ownerJoinsProject(t, fx.ID, restricted)
	dbfx.Exec(t, `UPDATE initiative SET build_project_id = $1 WHERE id = $2`, restricted, ini.ID)

	if got := fx.detail(t, ini.ID).Initiative.BuildProjectID; got == nil || *got != restricted {
		t.Fatalf("admin should see the build project %s, got %v", restricted, got)
	}
	var member InitiativeDetailResponse
	fx.detailAs(t, fx.MemberID, ini.ID).Want(http.StatusOK).JSON(&member)
	if member.Initiative.BuildProjectID != nil {
		t.Errorf("detail leaks the hidden build project %s", *member.Initiative.BuildProjectID)
	}
	found := false
	for _, v := range fx.listAs(t, fx.MemberID) {
		if v.ID == ini.ID {
			found = true
			if v.BuildProjectID != nil {
				t.Errorf("list leaks the hidden build project %s", *v.BuildProjectID)
			}
		}
	}
	if !found {
		t.Errorf("the initiative itself is visible and should be listed")
	}

	// A grant makes the project visible again.
	dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`, restricted, fx.ID, fx.MemberID)
	fx.detailAs(t, fx.MemberID, ini.ID).Want(http.StatusOK).JSON(&member)
	if member.Initiative.BuildProjectID == nil || *member.Initiative.BuildProjectID != restricted {
		t.Errorf("a granted member should see the build project, got %v", member.Initiative.BuildProjectID)
	}
}

func TestInitiativeVisibility_HiddenBackingIssueAnswersMissing(t *testing.T) {
	fx := newRoomedIniWorkspace(t)
	ini := fx.createInitiative(t, "HIDDEN-INITIATIVE-TITLE")
	restricted := dbfx.Project(t, "Restricted initiative "+uuid.NewString()[:8], testutil.Cols{"workspace_id": fx.ID, "visibility": "private"})
	ownerJoinsProject(t, fx.ID, restricted)
	dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = (SELECT issue_id FROM initiative WHERE id = $2)`, restricted, ini.ID)

	for _, v := range fx.listAs(t, fx.MemberID) {
		if v.ID == ini.ID {
			t.Errorf("list for a member without a grant includes the initiative %s", ini.ID)
		}
	}
	got := fx.detailAs(t, fx.MemberID, ini.ID)
	missing := fx.detailAs(t, fx.MemberID, uuid.NewString())
	if got.Code != http.StatusNotFound || got.Text() != missing.Text() {
		t.Errorf("hidden initiative should answer like a missing one: got %d %s, missing %d %s", got.Code, got.Text(), missing.Code, missing.Text())
	}
	found := false
	for _, v := range fx.listAs(t, fx.AdminID) {
		found = found || v.ID == ini.ID
	}
	if !found {
		t.Errorf("admin should still list the initiative")
	}
}
