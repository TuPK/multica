package handler

// Visibility of restricted projects on the issue and project read paths
// (spec 7.1 rules 3, 5, 6, 7). The fixture: one restricted project, one
// workspace project, a plain member who is not granted the restricted project
// (the outsider), and a plain member who is granted it (the grantee).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type restrictedFixture struct {
	restrictedProjectID string
	workspaceProjectID  string
	outsiderID          string
	granteeID           string
}

// addPlainMember creates a user and adds it to the test workspace as a member.
func addPlainMember(t *testing.T, label string) string {
	t.Helper()
	ctx := context.Background()
	email := "restricted-" + label + "-" + uuid.NewString()[:8] + "@multica.test"
	var userID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id`, label, email).Scan(&userID); err != nil {
		t.Fatalf("create %s user: %v", label, err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'member')`, testWorkspaceID, userID); err != nil {
		t.Fatalf("add %s as member: %v", label, err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM project_member WHERE user_id = $1`, userID)
		testPool.Exec(context.Background(), `DELETE FROM member WHERE user_id = $1`, userID)
		testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, userID)
	})
	return userID
}

func createRestrictedFixtureProject(t *testing.T, title, access string) string {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.CreateProject(w, newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{"title": title}))
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject %q: %d %s", title, w.Code, w.Body.String())
	}
	var p ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatalf("decode CreateProject: %v", err)
	}
	if access == "restricted" {
		makeProjectPrivate(t, p.ID)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM project_member WHERE project_id = $1`, p.ID)
		r := withURLParam(newRequest("DELETE", "/api/projects/"+p.ID, nil), "id", p.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), r)
	})
	return p.ID
}

func newRestrictedFixture(t *testing.T) restrictedFixture {
	t.Helper()
	f := restrictedFixture{
		restrictedProjectID: createRestrictedFixtureProject(t, "Restricted "+uuid.NewString()[:8], "restricted"),
		workspaceProjectID:  createRestrictedFixtureProject(t, "Open "+uuid.NewString()[:8], "workspace"),
		outsiderID:          addPlainMember(t, "outsider"),
		granteeID:           addPlainMember(t, "grantee"),
	}
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`,
		f.restrictedProjectID, testWorkspaceID, f.granteeID); err != nil {
		t.Fatalf("grant project: %v", err)
	}
	return f
}

// createIssueInProject creates an issue as the test owner, optionally in a
// project, and returns its id.
func createIssueInProject(t *testing.T, title, projectID string) string {
	t.Helper()
	body := map[string]any{"title": title, "status": "todo", "priority": "none"}
	if projectID != "" {
		body["project_id"] = projectID
	}
	w := httptest.NewRecorder()
	testHandler.CreateIssue(w, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIssue %q: %d %s", title, w.Code, w.Body.String())
	}
	var issue IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&issue); err != nil {
		t.Fatalf("decode CreateIssue: %v", err)
	}
	t.Cleanup(func() { deleteTestIssue(t, issue.ID) })
	return issue.ID
}

func requestAs(userID, method, path string, body any) *http.Request {
	req := newRequest(method, path, body)
	req.Header.Set("X-User-ID", userID)
	return req
}

func listIssueIDs(t *testing.T, userID string) (string, int64) {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.ListIssues(w, requestAs(userID, "GET", "/api/issues?workspace_id="+testWorkspaceID+"&limit=500", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListIssues as %s: %d %s", userID, w.Code, w.Body.String())
	}
	var out struct {
		Total int64 `json:"total"`
	}
	json.NewDecoder(strings.NewReader(w.Body.String())).Decode(&out)
	return w.Body.String(), out.Total
}

// TestRestrictedProjectHiddenFromEveryIssueRoute: the outsider gets 404 for a
// restricted project's issue on the single-issue route and does not see it in
// the list; the grantee and the owner do.
func TestRestrictedProjectHiddenFromEveryIssueRoute(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	issueID := createIssueInProject(t, "secret issue", f.restrictedProjectID)

	get := func(userID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		testHandler.GetIssue(w, withURLParam(requestAs(userID, "GET", "/api/issues/"+issueID, nil), "id", issueID))
		return w
	}
	if w := get(f.outsiderID); w.Code != http.StatusNotFound {
		t.Errorf("GetIssue as outsider: want 404, got %d %s", w.Code, w.Body.String())
	}
	if w := get(f.granteeID); w.Code != http.StatusOK {
		t.Errorf("GetIssue as grantee: want 200, got %d", w.Code)
	}
	if w := get(testUserID); w.Code != http.StatusOK {
		t.Errorf("GetIssue as owner: want 200, got %d", w.Code)
	}

	outsiderList, _ := listIssueIDs(t, f.outsiderID)
	if strings.Contains(outsiderList, issueID) {
		t.Errorf("ListIssues as outsider lists the restricted issue")
	}
	if granteeList, _ := listIssueIDs(t, f.granteeID); !strings.Contains(granteeList, issueID) {
		t.Errorf("ListIssues as grantee does not list the restricted issue")
	}

	w := httptest.NewRecorder()
	testHandler.ListProjects(w, requestAs(f.outsiderID, "GET", "/api/projects?workspace_id="+testWorkspaceID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListProjects as outsider: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), f.restrictedProjectID) {
		t.Errorf("ListProjects as outsider lists the restricted project")
	}
}

// TestProjectNotFoundLooksLikeMissing: a restricted project the caller may not
// see answers exactly like a project that does not exist, apart from the id.
func TestProjectNotFoundLooksLikeMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	missingID := uuid.NewString()

	getProject := func(id string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		testHandler.GetProject(w, withURLParam(requestAs(f.outsiderID, "GET", "/api/projects/"+id, nil), "id", id))
		return w
	}
	hidden := getProject(f.restrictedProjectID)
	missing := getProject(missingID)
	if hidden.Code != http.StatusNotFound || missing.Code != http.StatusNotFound {
		t.Fatalf("want 404 for both, got hidden=%d missing=%d", hidden.Code, missing.Code)
	}
	if strings.ReplaceAll(hidden.Body.String(), f.restrictedProjectID, "ID") !=
		strings.ReplaceAll(missing.Body.String(), missingID, "ID") {
		t.Errorf("hidden and missing project bodies differ:\nhidden:  %s\nmissing: %s",
			hidden.Body.String(), missing.Body.String())
	}
}

// TestCountsExcludeHiddenIssues: the list total for the outsider leaves out the
// one issue of the restricted project that the owner's total includes.
func TestCountsExcludeHiddenIssues(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	createIssueInProject(t, "hidden for outsider", f.restrictedProjectID)

	_, ownerTotal := listIssueIDs(t, testUserID)
	_, outsiderTotal := listIssueIDs(t, f.outsiderID)
	if outsiderTotal+1 != ownerTotal {
		t.Errorf("outsider total %d, owner total %d: want the outsider to miss exactly the hidden issue",
			outsiderTotal, ownerTotal)
	}
}

// TestAssigneeMustSeeProject: an issue of a restricted project cannot be
// assigned to a member who does not see the project.
func TestAssigneeMustSeeProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	w := httptest.NewRecorder()
	testHandler.CreateIssue(w, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":         "assigned to outsider",
		"status":        "todo",
		"priority":      "none",
		"project_id":    f.restrictedProjectID,
		"assignee_type": "member",
		"assignee_id":   f.outsiderID,
	}))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "project_member_invalid") {
		t.Errorf("want 400 project_member_invalid, got %d %s", w.Code, w.Body.String())
	}
}

// TestMoveIssueNeedsBothProjects: an issue cannot be moved into a restricted
// project by a caller who does not see that project; the answer is 404.
func TestMoveIssueNeedsBothProjects(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	issueID := createIssueInProject(t, "movable", f.workspaceProjectID)

	w := httptest.NewRecorder()
	req := withURLParam(requestAs(f.outsiderID, "POST", "/api/issues/"+issueID+"/move",
		map[string]any{"project_id": f.restrictedProjectID, "before_id": nil, "after_id": nil}), "id", issueID)
	testHandler.MoveIssue(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("MoveIssue into restricted project as outsider: want 400, got %d %s", w.Code, w.Body.String())
	}
}

// makeProjectPrivate marks a project private and, as creating one does, adds
// the test owner to it: a workspace role grants no visibility, so without a
// membership even the owner would not see it.
func makeProjectPrivate(t *testing.T, projectID string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE project SET visibility = 'private' WHERE id = $1`, projectID)
	var workspaceID string
	dbfx.QueryRow(t, `SELECT workspace_id::text FROM project WHERE id = $1`, projectID).Scan(&workspaceID)
	ownerJoinsProject(t, workspaceID, projectID)
}

// visibilityOf maps the fixtures' access vocabulary to a project visibility.
func visibilityOf(access string) string {
	if access == "restricted" {
		return "private"
	}
	return "workspace"
}

// ownerJoinsProject adds the workspace's owners and admins, and the test
// owner, to a private project's members, as creating the project would: a
// role grants no visibility, so without a row the owner side sees nothing.
func ownerJoinsProject(t *testing.T, workspaceID, projectID string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id)
		SELECT $1::uuid, $2::uuid, user_id FROM member WHERE workspace_id = $2::uuid AND role IN ('owner', 'admin')
		ON CONFLICT DO NOTHING`, projectID, workspaceID)
	dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, projectID, workspaceID, testUserID)
}
