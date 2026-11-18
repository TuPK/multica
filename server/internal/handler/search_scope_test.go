package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Scope tests for server search and the three index sync calls (spec 7.1
// rule 14, AC51). They run in the shared test workspace, so counts are
// compared between viewers rather than against absolute numbers.

func addWorkspaceAdmin(t *testing.T) string {
	t.Helper()
	userID := addPlainMember(t, "admin")
	if _, err := testPool.Exec(context.Background(),
		`UPDATE member SET role = 'admin' WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, userID); err != nil {
		t.Fatalf("make admin: %v", err)
	}
	return userID
}

func indexRequestAs(userID, method, path string, body any) *http.Request {
	req := requestAs(userID, method, path, body)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	return req
}

func manifestAs(t *testing.T, userID string) SearchIndexManifestResponse {
	t.Helper()
	return testutil.Decode[SearchIndexManifestResponse](t, testHandler.GetSearchIndexManifest,
		indexRequestAs(userID, http.MethodGet, "/api/search-index/manifest", nil), http.StatusOK)
}

func changesAs(t *testing.T, userID, cursor string) SearchIndexChangesResponse {
	t.Helper()
	return testutil.Decode[SearchIndexChangesResponse](t, testHandler.ListSearchIndexChanges,
		indexRequestAs(userID, http.MethodPost, "/api/search-index/changes", map[string]any{"cursor": cursor, "limit": 1000}),
		http.StatusOK)
}

// snapshotAs pages through the whole snapshot.
func snapshotAs(t *testing.T, userID string) SearchIndexSnapshotResponse {
	t.Helper()
	var all SearchIndexSnapshotResponse
	after := int32(0)
	for {
		page := testutil.Decode[SearchIndexSnapshotResponse](t, testHandler.GetSearchIndexSnapshot,
			indexRequestAs(userID, http.MethodGet, "/api/search-index/snapshot?limit=500&after_number="+strconv.Itoa(int(after)), nil), http.StatusOK)
		all.Issues = append(all.Issues, page.Issues...)
		all.Comments = append(all.Comments, page.Comments...)
		all.Projects = append(all.Projects, page.Projects...)
		if page.Done {
			return all
		}
		after = page.NextAfterNumber
	}
}

func TestSearchHidesRestrictedFromNonMembers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	adminID := addWorkspaceAdmin(t)
	token := "needlescope" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	dbfx.Exec(t, `UPDATE project SET title = $2 WHERE id = $1`, f.restrictedProjectID, "Secret "+token)
	// The issue title carries no token: only its comment can match.
	hidden := createIssueInProject(t, "Quiet title "+uuid.NewString()[:8], f.restrictedProjectID)
	dbfx.Comment(t, hidden, "comment with "+token, testutil.Cols{})

	search := func(h http.HandlerFunc, path, userID string) string {
		w := serveAs(h, requestAs(userID, "GET", path+"?workspace_id="+testWorkspaceID+"&include_closed=true&q="+url.QueryEscape(token), nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s as %s: %d %s", path, userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	issues := search(testHandler.SearchIssues, "/api/issues/search", f.outsiderID)
	projects := search(testHandler.SearchProjects, "/api/projects/search", f.outsiderID)
	if strings.Contains(issues, hidden) || strings.Contains(issues, "comment with "+token) {
		t.Errorf("non-member issue search returns the restricted issue or its comment: %s", issues)
	}
	if strings.Contains(projects, f.restrictedProjectID) || strings.Contains(projects, "Secret "+token) {
		t.Errorf("non-member project search returns the restricted project: %s", projects)
	}
	for name, userID := range map[string]string{"owner": testUserID, "admin": adminID, "granted member": f.granteeID} {
		if body := search(testHandler.SearchIssues, "/api/issues/search", userID); !strings.Contains(body, hidden) {
			t.Errorf("%s misses the restricted issue found through its comment: %s", name, body)
		}
		if body := search(testHandler.SearchProjects, "/api/projects/search", userID); !strings.Contains(body, f.restrictedProjectID) {
			t.Errorf("%s misses the restricted project: %s", name, body)
		}
	}
}

func TestManifestCountsOnlyVisible(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	adminID := addWorkspaceAdmin(t)
	before := map[string]SearchIndexManifestResponse{
		"outsider": manifestAs(t, f.outsiderID),
		"grantee":  manifestAs(t, f.granteeID),
		"owner":    manifestAs(t, testUserID),
		"admin":    manifestAs(t, adminID),
	}
	const title, body, comment = "Hidden count title", "hidden body", "hidden comment"
	hidden := createIssueInProject(t, title, f.restrictedProjectID)
	dbfx.Exec(t, `UPDATE issue SET description = $2 WHERE id = $1`, hidden, body)
	dbfx.Comment(t, hidden, comment, testutil.Cols{})
	open := createIssueInProject(t, "Open count title", f.workspaceProjectID)
	_ = open

	after := map[string]SearchIndexManifestResponse{
		"outsider": manifestAs(t, f.outsiderID),
		"grantee":  manifestAs(t, f.granteeID),
		"owner":    manifestAs(t, testUserID),
		"admin":    manifestAs(t, adminID),
	}
	hiddenBytes := int64(len(title) + len(body) + len(comment))
	openBytes := int64(len("Open count title"))
	wantIssues := map[string]int64{"outsider": 1, "grantee": 2, "owner": 2, "admin": 2}
	for name, a := range after {
		b := before[name]
		if got := a.IssueCount - b.IssueCount; got != wantIssues[name] {
			t.Errorf("%s: issue_count grew by %d, want %d", name, got, wantIssues[name])
		}
		wantComments := wantIssues[name] - 1
		if got := a.CommentCount - b.CommentCount; got != wantComments {
			t.Errorf("%s: comment_count grew by %d, want %d", name, got, wantComments)
		}
		wantBytes := openBytes
		if name != "outsider" {
			wantBytes += hiddenBytes
		}
		if got := a.TextBytes - b.TextBytes; got != wantBytes {
			t.Errorf("%s: text_bytes grew by %d, want %d", name, got, wantBytes)
		}
	}
	if got := after["owner"].ProjectCount - after["outsider"].ProjectCount; got != 1 {
		t.Errorf("owner sees %d more projects than the outsider, want 1 (the restricted one)", got)
	}
	if after["grantee"].ProjectCount != after["owner"].ProjectCount {
		t.Errorf("grantee project_count = %d, want %d", after["grantee"].ProjectCount, after["owner"].ProjectCount)
	}
}

func TestSnapshotAndChangesFilteredByScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	outsiderCursor := manifestAs(t, f.outsiderID).Cursor
	granteeCursor := manifestAs(t, f.granteeID).Cursor

	hidden := createIssueInProject(t, "Snapshot hidden", f.restrictedProjectID)
	open := createIssueInProject(t, "Snapshot open", f.workspaceProjectID)
	comment := dbfx.Comment(t, hidden, "snapshot hidden comment", testutil.Cols{})
	openComment := dbfx.Comment(t, open, "snapshot open comment", testutil.Cols{})

	hasIssue := func(issues []SearchIndexIssue, id string) bool {
		for _, is := range issues {
			if is.ID == id {
				return true
			}
		}
		return false
	}
	hasComment := func(comments []SearchIndexComment, id string) bool {
		for _, c := range comments {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	hasProject := func(projects []SearchIndexProject, id string) bool {
		for _, p := range projects {
			if p.ID == id {
				return true
			}
		}
		return false
	}

	snap := snapshotAs(t, f.outsiderID)
	if hasIssue(snap.Issues, hidden) || hasComment(snap.Comments, comment) || hasProject(snap.Projects, f.restrictedProjectID) {
		t.Errorf("outsider snapshot holds restricted rows")
	}
	if !hasIssue(snap.Issues, open) || !hasComment(snap.Comments, openComment) || !hasProject(snap.Projects, f.workspaceProjectID) {
		t.Errorf("outsider snapshot misses visible rows")
	}
	for name, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		snap := snapshotAs(t, userID)
		if !hasIssue(snap.Issues, hidden) || !hasComment(snap.Comments, comment) || !hasProject(snap.Projects, f.restrictedProjectID) {
			t.Errorf("%s snapshot misses restricted rows", name)
		}
	}

	ch := changesAs(t, f.outsiderID, outsiderCursor)
	if hasIssue(ch.Issues, hidden) || hasComment(ch.Comments, comment) || hasProject(ch.Projects, f.restrictedProjectID) {
		t.Errorf("outsider changes hold restricted rows")
	}
	if !hasIssue(ch.Issues, open) || !hasComment(ch.Comments, openComment) {
		t.Errorf("outsider changes miss visible rows")
	}
	ch = changesAs(t, f.granteeID, granteeCursor)
	if !hasIssue(ch.Issues, hidden) || !hasComment(ch.Comments, comment) {
		t.Errorf("grantee changes miss restricted rows")
	}
}

func TestScopeVersionChangesWithVisibleSet(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	v0 := manifestAs(t, f.outsiderID)
	if v0.ScopeVersion == "" {
		t.Fatal("manifest has no scope_version")
	}
	if again := manifestAs(t, f.outsiderID); again.ScopeVersion != v0.ScopeVersion {
		t.Errorf("scope_version moved without a change: %s -> %s", v0.ScopeVersion, again.ScopeVersion)
	}
	// Writes that do not change the visible set leave it alone.
	createIssueInProject(t, "Scope version noise", f.restrictedProjectID)
	if again := manifestAs(t, f.outsiderID); again.ScopeVersion != v0.ScopeVersion {
		t.Errorf("scope_version moved on an issue write: %s -> %s", v0.ScopeVersion, again.ScopeVersion)
	}

	dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`,
		f.restrictedProjectID, testWorkspaceID, f.outsiderID)
	granted := manifestAs(t, f.outsiderID)
	if granted.ScopeVersion == v0.ScopeVersion {
		t.Errorf("scope_version did not change after a grant")
	}
	if got := manifestAs(t, f.granteeID).ScopeVersion; got != granted.ScopeVersion {
		t.Errorf("two viewers with the same visible set differ: %s vs %s", got, granted.ScopeVersion)
	}

	// A cursor from before the grant is expired, so the client rebuilds.
	w := serveAs(testHandler.ListSearchIndexChanges, indexRequestAs(f.outsiderID, http.MethodPost,
		"/api/search-index/changes", map[string]any{"cursor": v0.Cursor, "limit": 10}))
	if w.Code != http.StatusGone {
		t.Errorf("changes with a pre-grant cursor = %d %s, want 410", w.Code, w.Body.String())
	}
	if w := serveAs(testHandler.ListSearchIndexChanges, indexRequestAs(f.outsiderID, http.MethodPost,
		"/api/search-index/changes", map[string]any{"cursor": granted.Cursor, "limit": 10})); w.Code != http.StatusOK {
		t.Errorf("changes with a current cursor = %d %s, want 200", w.Code, w.Body.String())
	}

	dbfx.Exec(t, `DELETE FROM project_member WHERE project_id = $1 AND user_id = $2`, f.restrictedProjectID, f.outsiderID)
	if revoked := manifestAs(t, f.outsiderID); revoked.ScopeVersion != v0.ScopeVersion {
		t.Errorf("scope_version after revoke = %s, want the pre-grant value %s", revoked.ScopeVersion, v0.ScopeVersion)
	}
	if w := serveAs(testHandler.ListSearchIndexChanges, indexRequestAs(f.outsiderID, http.MethodPost,
		"/api/search-index/changes", map[string]any{"cursor": granted.Cursor, "limit": 10})); w.Code != http.StatusGone {
		t.Errorf("changes with a pre-revoke cursor = %d, want 410", w.Code)
	}
}
