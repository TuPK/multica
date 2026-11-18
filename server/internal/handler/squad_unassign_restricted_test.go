package handler

// Unassigning a squad takes it off every project it works (spec 7.1 rules 3,
// 5). A member who manages the squad but does not see a restricted project
// leaves that project alone and is not told it exists.

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestRestrictedUnassignSquadProjectAnswersMissing: the outsider created the
// squad, which sits on a workspace project and on a restricted project. The
// unassign takes it off the workspace project only, counts only that project,
// and the restricted project keeps its squad. The owner then clears the rest.
func TestRestrictedUnassignSquadProjectAnswersMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	leaderID := createHandlerTestAgent(t, "Unassign Leader "+f.outsiderID[:8], nil)
	squadID := dbfx.Squad(t, "Unassign Squad "+f.outsiderID[:8], leaderID, testutil.Cols{"creator_id": f.outsiderID})
	for _, projectID := range []string{f.restrictedProjectID, f.workspaceProjectID} {
		dbfx.Exec(t, `UPDATE project SET squad_ids = ARRAY[$2]::uuid[] WHERE id = $1`, projectID, squadID)
	}

	unassign := func(userID string) *testutil.Response {
		req := newRequestAs(userID, "DELETE", "/api/squads/"+squadID+"/project", nil)
		req = withURLParams(req, "workspaceId", testWorkspaceID, "id", squadID)
		return testutil.Call(t, testHandler.UnassignSquadProject, req)
	}

	var out struct {
		Unassigned int `json:"unassigned"`
	}
	unassign(f.outsiderID).Want(http.StatusOK).JSON(&out)
	if out.Unassigned != 1 {
		t.Errorf("outsider unassigned = %d, want 1 (the workspace project only)", out.Unassigned)
	}
	if got := projectSquadIDs(t, f.restrictedProjectID); len(got) != 1 || got[0] != squadID {
		t.Errorf("restricted project squad_ids = %v after the outsider's unassign, want [%s]", got, squadID)
	}
	if got := projectSquadIDs(t, f.workspaceProjectID); len(got) != 0 {
		t.Errorf("workspace project squad_ids = %v, want none", got)
	}

	unassign(testUserID).Want(http.StatusOK).JSON(&out)
	if out.Unassigned != 1 {
		t.Errorf("owner unassigned = %d, want 1 (the restricted project)", out.Unassigned)
	}
	if got := projectSquadIDs(t, f.restrictedProjectID); len(got) != 0 {
		t.Errorf("restricted project squad_ids = %v after the owner's unassign, want none", got)
	}
}
