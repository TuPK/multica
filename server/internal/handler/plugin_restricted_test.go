package handler

// Visibility of restricted projects through the plugin bridge (spec 7.1 rule
// 3): a plugin call made for a user reaches only what that user sees, and a
// hidden issue answers like a missing one.

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var problemRequestID = regexp.MustCompile(`"request_id":"[^"]*"`)

func TestRestrictedPluginIssueAnswersMissing(t *testing.T) {
	installationID := installPluginForAction(t, []string{"issues:read", "issues:write"})
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "plugin hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	missingID := uuid.NewString()
	prefix := testHandler.getIssuePrefix(t.Context(), parseUUID(testWorkspaceID))
	hiddenIdent := prefix + "-" + strconv.Itoa(issueNumber(t, hiddenID))
	missingIdent := prefix + "-999999"

	calls := map[string]struct {
		handler http.HandlerFunc
		method  string
		body    any
	}{
		"GetPluginIssue":   {testHandler.GetPluginIssue, http.MethodGet, nil},
		"PatchPluginIssue": {testHandler.PatchPluginIssue, http.MethodPatch, map[string]any{"title": "changed through the plugin"}},
	}
	for name, c := range calls {
		call := func(userID, ref string) *httptest.ResponseRecorder {
			req := pluginActionRequest(c.method, "/v1/issues/"+ref, installationID, c.body, map[string]string{"issue_ref": ref})
			req.Header.Set("X-User-ID", userID)
			return serveAs(c.handler, req)
		}
		for label, refs := range map[string][2]string{"uuid": {hiddenID, missingID}, "identifier": {hiddenIdent, missingIdent}} {
			hidden, missing := call(f.outsiderID, refs[0]), call(f.outsiderID, refs[1])
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s by %s: a missing issue answers %d, want 404", name, label, missing.Code)
			}
			normalize := func(body, ref string) string {
				return problemRequestID.ReplaceAllString(strings.ReplaceAll(body, ref, "REF"), `"request_id":""`)
			}
			if hidden.Code != missing.Code ||
				normalize(hidden.Body.String(), refs[0]) != normalize(missing.Body.String(), refs[1]) {
				t.Errorf("%s by %s as outsider: hidden answers %d %s, missing answers %d %s",
					name, label, hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
			}
		}
		if w := call(f.granteeID, hiddenID); w.Code != http.StatusOK {
			t.Errorf("%s as grantee: want 200, got %d %s", name, w.Code, w.Body.String())
		}
		if w := call(testUserID, hiddenID); w.Code != http.StatusOK {
			t.Errorf("%s as owner: want 200, got %d %s", name, w.Code, w.Body.String())
		}
	}

	var title string
	dbfx.QueryRow(t, `SELECT title FROM issue WHERE id = $1`, hiddenID).Scan(&title)
	if title != "changed through the plugin" {
		t.Errorf("the owner's patch did not apply: title %q", title)
	}
}
