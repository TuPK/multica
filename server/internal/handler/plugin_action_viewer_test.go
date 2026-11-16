package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	publicapiv1 "github.com/multica-ai/multica/server/pkg/publicapi/v1"
)

// Spec 6.6: through the plugin bridge a viewer reads but writes nothing.
// pluginSessionCaller refuses a viewer's non-read method after the member
// lookup, so the request has to carry a real installation of the workspace to
// get that far. The installer is the workspace owner; the viewer and the
// member are separate users of the same workspace.
func TestPluginBridgeRefusesAViewerWrites(t *testing.T) {
	installationID := installPluginForAction(t, []string{"issues:read", "issues:write", "comments:read", "comments:write"})
	issueID := createTestIssue(t, "Viewer bridge issue", "todo", "none")

	viewerID := dbfx.User(t, "Bridge Viewer", testutil.UniqueEmail("bridge-viewer"))
	dbfx.Member(t, testWorkspaceID, viewerID, workspaceguard.RoleViewer)
	memberID := dbfx.User(t, "Bridge Member", testutil.UniqueEmail("bridge-member"))
	dbfx.Member(t, testWorkspaceID, memberID, "member")

	issuePath := "/api/plugin-bridge/v1/issues/" + issueID
	params := map[string]string{"issue_ref": issueID}
	patchBody := map[string]any{"title": "Changed through the bridge"}
	commentBody := map[string]any{"content": "written through the bridge"}

	call := func(userID string, handle http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
		request := pluginActionRequest(method, path, installationID, body, params)
		request.Header.Set("X-User-ID", userID)
		rec := httptest.NewRecorder()
		handle(rec, request)
		return rec
	}
	problemCode := func(rec *httptest.ResponseRecorder) string {
		var problem publicapiv1.Problem
		_ = json.Unmarshal(rec.Body.Bytes(), &problem)
		return problem.Code
	}
	refusedAsViewer := func(rec *httptest.ResponseRecorder) bool {
		return rec.Code == http.StatusForbidden && problemCode(rec) == workspaceguard.CodeViewerReadOnly
	}

	var titleBefore string
	dbfx.QueryRow(t, `SELECT title FROM issue WHERE id = $1`, issueID).Scan(&titleBefore)

	t.Run("viewer PATCH is refused", func(t *testing.T) {
		rec := call(viewerID, testHandler.PatchPluginIssue, http.MethodPatch, issuePath, patchBody)
		if !refusedAsViewer(rec) {
			t.Fatalf("status=%d body=%s, want 403 %s", rec.Code, rec.Body.String(), workspaceguard.CodeViewerReadOnly)
		}
		var titleAfter string
		dbfx.QueryRow(t, `SELECT title FROM issue WHERE id = $1`, issueID).Scan(&titleAfter)
		if titleAfter != titleBefore {
			t.Fatalf("issue title changed to %q by a refused viewer", titleAfter)
		}
	})

	t.Run("viewer POST comment is refused", func(t *testing.T) {
		rec := call(viewerID, testHandler.CreatePluginComment, http.MethodPost, issuePath+"/comments", commentBody)
		if !refusedAsViewer(rec) {
			t.Fatalf("status=%d body=%s, want 403 %s", rec.Code, rec.Body.String(), workspaceguard.CodeViewerReadOnly)
		}
		var comments int
		dbfx.QueryRow(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, issueID).Scan(&comments)
		if comments != 0 {
			t.Fatalf("%d comments written by a refused viewer", comments)
		}
	})

	t.Run("viewer GET is not refused", func(t *testing.T) {
		rec := call(viewerID, testHandler.GetPluginIssue, http.MethodGet, issuePath, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
		}
	})

	t.Run("member PATCH is not refused", func(t *testing.T) {
		rec := call(memberID, testHandler.PatchPluginIssue, http.MethodPatch, issuePath, patchBody)
		if refusedAsViewer(rec) {
			t.Fatalf("member refused as a viewer: %s", rec.Body.String())
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
		}
	})
}
