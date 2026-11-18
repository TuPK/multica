package handler

// AC61 / P41: a workspace whose projects all have access=workspace and that
// has no guests gives the same list, search, count and event result as before
// restricted projects existed. Assertions are on this test's own rows.

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestAllWorkspaceProjectsBehaveAsBefore(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	member := addPlainMember(t, "asbefore-member")
	token := "asbefore" + uuid.NewString()[:8]

	projA := dbfx.Project(t, "A "+token)
	projB := dbfx.Project(t, "B "+token)
	for _, p := range []string{projA, projB} {
		var access string
		dbfx.QueryRow(t, `SELECT access FROM project WHERE id = $1`, p).Scan(&access)
		if access != "workspace" {
			t.Fatalf("default project access = %q, want workspace", access)
		}
	}
	issueA1 := dbfx.Issue(t, "one "+token, testutil.Cols{"project_id": projA})
	issueA2 := dbfx.Issue(t, "two "+token, testutil.Cols{"project_id": projA})
	issueB := dbfx.Issue(t, "three "+token, testutil.Cols{"project_id": projB})
	issueNone := dbfx.Issue(t, "four "+token)
	all := []string{issueA1, issueA2, issueB, issueNone}

	for name, uid := range map[string]string{"member": member, "owner": testUserID} {
		t.Run("list_"+name, func(t *testing.T) {
			var out rfeIssueList
			path := "/api/issues?workspace_id=" + testWorkspaceID + "&limit=500"
			testutil.Call(t, testHandler.ListIssues, requestAs(uid, "GET", path, nil)).Want(http.StatusOK).JSON(&out)
			for _, id := range all {
				if !out.has(id) {
					t.Errorf("%s's list lacks issue %s", name, id)
				}
			}
		})
		t.Run("search_"+name, func(t *testing.T) {
			got := map[string]bool{}
			for _, i := range rfeSearch(t, uid, token) {
				got[i.ID] = true
			}
			for _, id := range all {
				if !got[id] {
					t.Errorf("%s's search lacks issue %s", name, id)
				}
			}
		})
		t.Run("count_"+name, func(t *testing.T) {
			if got := rfeList(t, uid, projA).Total; got != 2 {
				t.Errorf("%s's count in project A = %d, want 2", name, got)
			}
			if got := rfeList(t, uid, projB).Total; got != 1 {
				t.Errorf("%s's count in project B = %d, want 1", name, got)
			}
		})
	}

	t.Run("event", func(t *testing.T) {
		for _, p := range []string{projA, projB, ""} {
			a := rfeAudience(t, p)
			if !a.Workspace || len(a.UserIDs) != 0 {
				t.Errorf("audience of project %q = %+v, want Workspace with no user ids", p, a)
			}
		}
	})
}
