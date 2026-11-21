package handler

// A squad works one open project at a time (spec 7.1 rule 2). The conflict
// check counts a restricted project the caller cannot see, so a second open
// project is still refused; the 409 names no project, so a caller who cannot
// see the open one learns nothing about it.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestOpenSquadProjectConflictHidesProjectDetails(t *testing.T) {
	fx := newIniWorkspace(t)
	squadID := sdlcSquad(t, fx, "Open conflict")
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, squadID, fx.MemberID)
	target := dbfxProject(t, fx, "Target")
	hiddenProject := dbfxProject(t, fx, "Hidden open")
	dbfx.Exec(t, `UPDATE project SET visibility = 'private', squad_ids = ARRAY[$2::uuid] WHERE id = $1`, hiddenProject, squadID)

	assign := func(userID string) *testutil.Response {
		req := newRequestAs(userID, "POST", "/api/squads/"+squadID+"/project", map[string]any{"project_id": target})
		return testutil.Call(t, testHandler.AssignSquadProject, withURLParams(req, "workspaceId", fx.ID, "id", squadID))
	}

	// The creator does not see the restricted project and is still refused:
	// hiding it from the conflict check would let the squad hold two open projects.
	resp := assign(fx.MemberID).Want(http.StatusConflict)
	if strings.Contains(resp.Body.String(), hiddenProject) || strings.Contains(resp.Body.String(), "Hidden open") {
		t.Errorf("409 for a hidden open project names it: %s", resp.Body.String())
	}
	if got := projectSquadIDs(t, target); len(got) != 0 {
		t.Errorf("refused assignment added the squad to the target project: %v", got)
	}
	// The owner sees the same 409 and the same empty body detail.
	ownerResp := assign(fx.AdminID).Want(http.StatusConflict)
	if strings.Contains(ownerResp.Body.String(), hiddenProject) {
		t.Errorf("owner 409 names the hidden project: %s", ownerResp.Body.String())
	}
}
