package handler

// An install token acting as the plugin has no viewer. It must not keep
// workspace-wide reach: issues in restricted projects answer 404 exactly as a
// missing issue does (spec 7.1 rule 5).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPluginInstallTokenCannotReachRestrictedProjectIssue(t *testing.T) {
	installationID := installPluginForAction(t, []string{"issues:read", "issues:write"})
	token, err := testHandler.PluginService.IssueInstallToken(context.Background(), parseUUID(installationID))
	if err != nil {
		t.Fatalf("issue install token: %v", err)
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "install token hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	visibleID := createIssueInProject(t, "install token visible "+uuid.NewString()[:8], f.workspaceProjectID)
	var originalTitle string
	dbfx.QueryRow(t, `SELECT title FROM issue WHERE id = $1`, hiddenID).Scan(&originalTitle)

	call := func(handler http.HandlerFunc, method, ref string, body any) *httptest.ResponseRecorder {
		req := pluginInstallTokenRequest(method, "/v1/issues/"+ref, token, body, map[string]string{"issue_ref": ref})
		return serveAs(handler, req)
	}

	t.Run("read answers like a missing issue", func(t *testing.T) {
		hidden := call(testHandler.GetPluginIssue, http.MethodGet, hiddenID, nil)
		missing := call(testHandler.GetPluginIssue, http.MethodGet, uuid.NewString(), nil)
		if hidden.Code != http.StatusNotFound {
			t.Fatalf("hidden issue read = %d %s, want 404", hidden.Code, hidden.Body.String())
		}
		if !strings.Contains(hidden.Body.String(), "not_found") || !strings.Contains(hidden.Body.String(), "issue not found") {
			t.Errorf("hidden issue body = %s, want not_found / issue not found", hidden.Body.String())
		}
		if missing.Code != hidden.Code {
			t.Errorf("missing issue answers %d, hidden answers %d", missing.Code, hidden.Code)
		}
	})

	t.Run("patch does not change the hidden issue", func(t *testing.T) {
		w := call(testHandler.PatchPluginIssue, http.MethodPatch, hiddenID, map[string]any{"title": "changed by install token"})
		if w.Code != http.StatusNotFound {
			t.Fatalf("hidden issue patch = %d %s, want 404", w.Code, w.Body.String())
		}
		var title string
		dbfx.QueryRow(t, `SELECT title FROM issue WHERE id = $1`, hiddenID).Scan(&title)
		if title != originalTitle {
			t.Errorf("hidden issue title changed to %q", title)
		}
	})

	t.Run("workspace-access project issue still works", func(t *testing.T) {
		if w := call(testHandler.GetPluginIssue, http.MethodGet, visibleID, nil); w.Code != http.StatusOK {
			t.Fatalf("visible issue read = %d %s, want 200", w.Code, w.Body.String())
		}
		if w := call(testHandler.PatchPluginIssue, http.MethodPatch, visibleID, map[string]any{"title": "visible patched"}); w.Code != http.StatusOK {
			t.Fatalf("visible issue patch = %d %s, want 200", w.Code, w.Body.String())
		}
	})
}
