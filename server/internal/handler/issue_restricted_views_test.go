package handler

// Visibility of restricted projects on saved views, view preferences and the
// trigger preview (spec 7.1 rules 3 and 5): a hidden project or issue answers
// like a missing one and nothing is written.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// TestRestrictedIssueViewProjectAnswersMissing: creating a view, reading a
// preference and writing a preference for a project the outsider cannot see
// answer like the same call for a missing project, and write nothing.
func TestRestrictedIssueViewProjectAnswersMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	missingID := uuid.NewString()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_view_preference WHERE scope_id = $1`, f.restrictedProjectID)
		testPool.Exec(context.Background(), `DELETE FROM issue_view WHERE scope_id = $1`, f.restrictedProjectID)
	})

	calls := map[string]func(userID, projectID string) *httptest.ResponseRecorder{
		"CreateIssueView": func(userID, projectID string) *httptest.ResponseRecorder {
			return serveAs(testHandler.CreateIssueView, requestAs(userID, "POST", "/api/issue-views", map[string]any{
				"name": "restricted view", "scope_type": "project", "scope_id": projectID, "visibility": "private",
				"query": map[string]any{}, "display": map[string]any{},
			}))
		},
		"GetIssueViewPreference": func(userID, projectID string) *httptest.ResponseRecorder {
			return serveAs(testHandler.GetIssueViewPreference, requestAs(userID, "GET",
				"/api/issue-view-preferences?scope_type=project&scope_id="+projectID, nil))
		},
		"PutIssueViewPreference": func(userID, projectID string) *httptest.ResponseRecorder {
			return serveAs(testHandler.PutIssueViewPreference, requestAs(userID, "PUT", "/api/issue-view-preferences", map[string]any{
				"scope_type": "project", "scope_id": projectID, "prefs": map[string]any{"viewMode": "list"},
			}))
		},
	}
	for name, call := range calls {
		hidden, missing := call(f.outsiderID, f.restrictedProjectID), call(f.outsiderID, missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("%s: a missing project answers %d %s, want 404", name, missing.Code, missing.Body.String())
		}
		if hidden.Code != missing.Code ||
			strings.ReplaceAll(hidden.Body.String(), f.restrictedProjectID, "ID") != strings.ReplaceAll(missing.Body.String(), missingID, "ID") {
			t.Errorf("%s as outsider: hidden project answers %d %s, missing answers %d %s",
				name, hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
		}
	}
	if n := countRows(t, `SELECT count(*) FROM issue_view WHERE scope_id = $1`, f.restrictedProjectID); n != 0 {
		t.Errorf("outsider created %d views on a hidden project", n)
	}
	if n := countRows(t, `SELECT count(*) FROM issue_view_preference WHERE scope_id = $1`, f.restrictedProjectID); n != 0 {
		t.Errorf("outsider wrote %d preferences on a hidden project", n)
	}
	for name, call := range calls {
		for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
			if w := call(userID, f.restrictedProjectID); w.Code != http.StatusOK && w.Code != http.StatusCreated {
				t.Errorf("%s as %s: want 200 or 201, got %d %s", name, who, w.Code, w.Body.String())
			}
		}
	}
}

// TestRestrictedPreviewTriggerAnswersMissing: the trigger preview of a hidden
// issue gives the outsider the answer it gives for a missing issue, and the
// owner sees the run it would start.
func TestRestrictedPreviewTriggerAnswersMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "preview hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	missingID := uuid.NewString()
	agentID := dbfx.Agent(t, "restricted preview "+uuid.NewString()[:8], testRuntimeID, testutil.Cols{"visibility": "workspace", "permission_mode": "public_to"})
	dbfx.Insert(t, "agent_invocation_target", testutil.Cols{"agent_id": agentID, "target_type": "workspace", "target_id": testWorkspaceID})
	dbfx.Exec(t, `UPDATE issue SET status = 'backlog', assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, hiddenID, agentID)

	preview := func(userID, issueID string) *httptest.ResponseRecorder {
		return serveAs(testHandler.PreviewIssueTrigger, requestAs(userID, "POST",
			"/api/issues/preview-trigger?workspace_id="+testWorkspaceID, map[string]any{
				"issue_ids": []string{issueID}, "status": "todo",
			}))
	}
	hidden, missing := preview(f.outsiderID, hiddenID), preview(f.outsiderID, missingID)
	if hidden.Code != missing.Code ||
		strings.ReplaceAll(hidden.Body.String(), hiddenID, "ID") != strings.ReplaceAll(missing.Body.String(), missingID, "ID") {
		t.Errorf("outsider: hidden issue answers %d %s, missing answers %d %s",
			hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
	}
	for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if w := preview(userID, hiddenID); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), hiddenID) {
			t.Errorf("%s: want a trigger for the issue, got %d %s", who, w.Code, w.Body.String())
		}
	}
}
