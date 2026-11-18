package handler

// P41.10: one restricted project through its whole life. Every step asserts on
// the rows this test created (project, issue, comment, two members); the
// database is shared with other tests.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	"github.com/multica-ai/multica/server/internal/testutil"
)

type rfeIssueList struct {
	Issues []IssueResponse `json:"issues"`
	Total  int64           `json:"total"`
}

func (l rfeIssueList) has(id string) bool {
	for _, i := range l.Issues {
		if i.ID == id {
			return true
		}
	}
	return false
}

func rfeList(t *testing.T, userID, projectID string) rfeIssueList {
	t.Helper()
	var out rfeIssueList
	path := "/api/issues?workspace_id=" + testWorkspaceID + "&limit=200&project_id=" + projectID
	testutil.Call(t, testHandler.ListIssues, requestAs(userID, "GET", path, nil)).Want(http.StatusOK).JSON(&out)
	return out
}

func rfeSearch(t *testing.T, userID, q string) []SearchIssueResponse {
	t.Helper()
	var out struct {
		Issues []SearchIssueResponse `json:"issues"`
	}
	path := "/api/issues/search?workspace_id=" + testWorkspaceID + "&q=" + url.QueryEscape(q)
	testutil.Call(t, testHandler.SearchIssues, requestAs(userID, "GET", path, nil)).Want(http.StatusOK).JSON(&out)
	return out.Issues
}

func rfeGet(userID, issueID string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	testHandler.GetIssue(w, withURLParam(requestAs(userID, "GET", "/api/issues/"+issueID, nil), "id", issueID))
	return w
}

func rfeAudience(t *testing.T, projectID string) projectaccess.Recipients {
	t.Helper()
	var pid pgtype.UUID // invalid for an issue without a project
	if projectID != "" {
		pid = parseUUID(projectID)
	}
	r, err := projectaccess.Audience(context.Background(), testHandler.Queries, parseUUID(testWorkspaceID), pid)
	if err != nil {
		t.Fatalf("Audience: %v", err)
	}
	return r
}

func rfeContains(ids []string, id string) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func TestRestrictedFlowEndToEnd(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	access := projectaccess.NewHandler(testHandler.Queries, testPool)
	m := addPlainMember(t, "flow-m")
	u := addPlainMember(t, "flow-u")
	token := "flowtok" + uuid.NewString()[:8]
	title := "Flow issue " + token

	parent := t
	var projectID, issueID string

	putAccess := func(t *testing.T, mode string) {
		t.Helper()
		req := withURLParam(newRequest("PUT", "/api/projects/"+projectID+"/access", map[string]any{"access": mode, "member_ids": []string{}}), "id", projectID)
		testutil.Call(t, access.PutAccess, req).Want(http.StatusOK)
	}

	t.Run("create", func(t *testing.T) {
		var p ProjectResponse
		testutil.Call(t, testHandler.CreateProject,
			newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{"title": "Flow " + token})).
			Want(http.StatusCreated).JSON(&p)
		projectID = p.ID
		// Registered on the parent: a subtest's own cleanup runs when it ends.
		parent.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM project_member WHERE project_id = $1`, projectID)
			testHandler.DeleteProject(httptest.NewRecorder(), withURLParam(newRequest("DELETE", "/api/projects/"+projectID, nil), "id", projectID))
		})
		putAccess(t, "restricted")
		var got string
		dbfx.QueryRow(t, `SELECT access FROM project WHERE id = $1`, projectID).Scan(&got)
		if got != "restricted" {
			t.Fatalf("project access = %q, want restricted", got)
		}
	})
	if projectID == "" {
		t.Fatal("project not created")
	}

	t.Run("grant", func(t *testing.T) {
		req := withURLParam(newRequest("POST", "/api/projects/"+projectID+"/members", map[string]any{"user_ids": []string{m}}), "id", projectID)
		testutil.Call(t, access.AddMembers, req).Want(http.StatusOK)
		if n := dbfx.Count(t, `SELECT count(*) FROM project_member WHERE project_id = $1 AND user_id = $2`, projectID, m); n != 1 {
			t.Fatalf("grant rows for M = %d, want 1", n)
		}
	})

	t.Run("read_and_create", func(t *testing.T) {
		var issue IssueResponse
		testutil.Call(t, testHandler.CreateIssue, requestAs(m, "POST", "/api/issues?workspace_id="+testWorkspaceID,
			map[string]any{"title": title, "status": "todo", "priority": "none", "project_id": projectID})).
			Want(http.StatusCreated).JSON(&issue)
		issueID = issue.ID
		parent.Cleanup(func() { deleteTestIssue(parent, issueID) })

		testutil.Call(t, testHandler.CreateComment,
			withURLParam(requestAs(m, "POST", "/api/issues/"+issueID+"/comments", map[string]any{"content": "comment from M " + token}), "id", issueID)).
			Want(http.StatusCreated)
		if n := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, issueID); n != 1 {
			t.Errorf("comments on issue = %d, want 1", n)
		}
		for name, uid := range map[string]string{"M": m, "owner": testUserID} {
			if w := rfeGet(uid, issueID); w.Code != http.StatusOK {
				t.Errorf("GetIssue as %s: %d %s", name, w.Code, w.Body.String())
			}
		}
		if w := rfeGet(u, issueID); w.Code != http.StatusNotFound {
			t.Errorf("GetIssue as U: want 404, got %d", w.Code)
		}
	})
	if issueID == "" {
		t.Fatal("issue not created")
	}

	t.Run("list", func(t *testing.T) {
		if !rfeList(t, m, projectID).has(issueID) {
			t.Errorf("M's list lacks the issue")
		}
		if rfeList(t, u, projectID).has(issueID) {
			t.Errorf("U's list contains the issue")
		}
	})

	t.Run("search", func(t *testing.T) {
		found := func(uid string) bool {
			for _, i := range rfeSearch(t, uid, token) {
				if i.ID == issueID {
					return true
				}
			}
			return false
		}
		if !found(m) {
			t.Errorf("M's search does not find the issue")
		}
		if found(u) {
			t.Errorf("U's search finds the issue")
		}
	})

	t.Run("count", func(t *testing.T) {
		if got := rfeList(t, m, projectID).Total; got != 1 {
			t.Errorf("M's count = %d, want 1", got)
		}
		if got := rfeList(t, u, projectID).Total; got != 0 {
			t.Errorf("U's count = %d, want 0", got)
		}
	})

	t.Run("event", func(t *testing.T) {
		// The handler publishes through the bus, which the test handler does not
		// capture; this asserts the audience those events are routed with.
		a := rfeAudience(t, projectID)
		if a.Workspace {
			t.Errorf("restricted project audience has Workspace=true")
		}
		if !rfeContains(a.UserIDs, m) {
			t.Errorf("M missing from audience %v", a.UserIDs)
		}
		if !rfeContains(a.UserIDs, testUserID) {
			t.Errorf("owner missing from audience %v", a.UserIDs)
		}
		if rfeContains(a.UserIDs, u) {
			t.Errorf("U present in audience %v", a.UserIDs)
		}
	})

	t.Run("member_removal", func(t *testing.T) {
		req := withURLParams(newRequest("DELETE", "/api/projects/"+projectID+"/members/"+m, nil), "id", projectID, "userId", m)
		testutil.Call(t, access.RemoveMember, req).Want(http.StatusNoContent)
		if w := rfeGet(m, issueID); w.Code != http.StatusNotFound {
			t.Errorf("GetIssue as M after removal: want 404, got %d %s", w.Code, w.Body.String())
		}
		if rfeList(t, m, projectID).has(issueID) {
			t.Errorf("M's list still has the issue")
		}
		if a := rfeAudience(t, projectID); rfeContains(a.UserIDs, m) {
			t.Errorf("M still in audience %v", a.UserIDs)
		}
	})

	t.Run("reopen_to_workspace", func(t *testing.T) {
		putAccess(t, "workspace")
		if !rfeList(t, u, projectID).has(issueID) {
			t.Errorf("U's list lacks the issue after reopening")
		}
		// Switching to restricted auto-grants the project's creator (the owner),
		// and no switch removes a grant, so that row stays. No guest or
		// member grant of this test remains.
		if n := dbfx.Count(t, `SELECT count(*) FROM project_member WHERE project_id = $1 AND user_id <> $2`, projectID, testUserID); n != 0 {
			t.Errorf("project_member rows other than the owner's = %d, want 0", n)
		}
		if a := rfeAudience(t, projectID); !a.Workspace || rfeContains(a.UserIDs, m) || rfeContains(a.UserIDs, u) {
			t.Errorf("audience after reopen = %+v, want Workspace without M or U", a)
		}
	})
}
