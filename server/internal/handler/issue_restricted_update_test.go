package handler

// Every viewer-triggered route that reaches the UpdateIssue query (spec 7.1
// rules 3, 5). The outsider is a plain member not granted the restricted
// project; a hidden issue must answer like a missing one and stay unchanged.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestRestrictedUpdateCallersAnswerMissing: the issue update, the move, the
// batch update, the flow task close, the initiative transitions and the plugin
// patch each answer the outsider like the same call on an issue that does not
// exist, and the hidden issue row does not change.
func TestRestrictedUpdateCallersAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	t.Run("issue routes", func(t *testing.T) {
		f := newRestrictedFixture(t)
		hiddenID := createIssueInProject(t, "update caller hidden "+uuid.NewString()[:8], f.restrictedProjectID)
		missingID := uuid.NewString()
		before := issueRowSnapshot(t, hiddenID)
		routes := []struct {
			name    string
			handler http.HandlerFunc
			method  string
			path    string
			body    any
		}{
			{"UpdateIssue", testHandler.UpdateIssue, "PUT", "/api/issues/{id}", map[string]any{"title": "hacked", "status": "done"}},
			{"MoveIssue", testHandler.MoveIssue, "POST", "/api/issues/{id}/move", map[string]any{"status": "done"}},
		}
		for _, route := range routes {
			call := func(id string) *httptest.ResponseRecorder {
				req := requestAs(f.outsiderID, route.method, strings.ReplaceAll(route.path, "{id}", id), route.body)
				return serveAs(route.handler, withURLParam(req, "id", id))
			}
			missing := call(missingID)
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s on a missing issue: want 404, got %d %s", route.name, missing.Code, missing.Body.String())
			}
			sameAnswer(t, route.name+" as outsider", call(hiddenID), missing, hiddenID, missingID)
		}
		// The batch route skips an id it cannot find; a hidden id is skipped alike.
		batch := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.BatchUpdateIssues, requestAs(f.outsiderID, "POST",
				"/api/issues/batch-update?workspace_id="+testWorkspaceID,
				map[string]any{"issue_ids": []string{id}, "updates": map[string]any{"title": "hacked", "status": "done"}}))
		}
		sameAnswer(t, "BatchUpdateIssues as outsider", batch(hiddenID), batch(missingID), hiddenID, missingID)
		if after := issueRowSnapshot(t, hiddenID); after != before {
			t.Errorf("an update of the outsider changed the hidden issue:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	// The flow close runs UpdateIssue on the task the URL names.
	t.Run("flow task close", func(t *testing.T) {
		f := newRestrictedFixture(t)
		fx := newFlowFixture(t)
		dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, f.restrictedProjectID, fx.Work)
		missingID := uuid.NewString()
		before := issueRowSnapshot(t, fx.Work)
		call := func(id string) *httptest.ResponseRecorder {
			req := requestAs(f.outsiderID, "POST", "/api/issues/"+id+"/flow/close", map[string]any{"verdict": "ok"})
			return serveAs(testHandler.CloseIssueFlowTask, withURLParams(req, "id", id))
		}
		missing := call(missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("CloseIssueFlowTask on a missing issue: want 404, got %d %s", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "CloseIssueFlowTask as outsider", call(fx.Work), missing, fx.Work, missingID)
		if after := issueRowSnapshot(t, fx.Work); after != before {
			t.Errorf("a flow close of the outsider changed the hidden task:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	// An initiative transition moves the backing issue and hands it to the stage owner.
	t.Run("initiative transition", func(t *testing.T) {
		fx := newRoomedIniWorkspace(t)
		restricted := dbfx.Project(t, "Restricted initiative "+uuid.NewString()[:8], testutil.Cols{"workspace_id": fx.ID, "visibility": "private"})
		dbfx.SquadMember(t, fx.Product, "member", fx.MemberID, testutil.Cols{"role": "member"})
		created := fx.createInitiative(t, "Hidden update initiative")
		var issueID string
		dbfx.QueryRow(t, `SELECT issue_id FROM initiative WHERE id = $1`, created.ID).Scan(&issueID)
		dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, restricted, issueID)
		fx.linkStage(t, created.ID, "validation_report", "competitor_profile")
		fx.makeAutoExit(t, 0)
		missingID := uuid.NewString()
		before := issueRowSnapshot(t, issueID)
		call := func(id string) *httptest.ResponseRecorder {
			req := fx.as(newRequest("POST", "/api/initiatives/"+id, map[string]any{"note": "go"}), fx.MemberID)
			return serveAs(testHandler.AdvanceInitiative, withURLParams(req, "id", id))
		}
		missing := call(missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("AdvanceInitiative on a missing initiative: want 404, got %d %s", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "AdvanceInitiative as outsider", call(created.ID), missing, created.ID, missingID)
		if after := issueRowSnapshot(t, issueID); after != before {
			t.Errorf("an initiative write of the outsider changed the backing issue:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	// IssueService.UpdateContent runs UpdateIssue for the plugin bridge.
	t.Run("plugin patch", func(t *testing.T) {
		installationID := installPluginForAction(t, []string{"issues:read", "issues:write"})
		f := newRestrictedFixture(t)
		hiddenID := createIssueInProject(t, "plugin update hidden "+uuid.NewString()[:8], f.restrictedProjectID)
		missingID := uuid.NewString()
		before := issueRowSnapshot(t, hiddenID)
		call := func(ref string) *httptest.ResponseRecorder {
			req := pluginActionRequest(http.MethodPatch, "/v1/issues/"+ref, installationID,
				map[string]any{"title": "changed through the plugin"}, map[string]string{"issue_ref": ref})
			req.Header.Set("X-User-ID", f.outsiderID)
			return serveAs(testHandler.PatchPluginIssue, req)
		}
		missing := call(missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("PatchPluginIssue on a missing issue: want 404, got %d %s", missing.Code, missing.Body.String())
		}
		hidden := call(hiddenID)
		if hidden.Code != missing.Code {
			t.Errorf("PatchPluginIssue as outsider: hidden answers %d %s, missing answers %d %s",
				hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
		}
		hiddenByIdent := call(testHandler.getIssuePrefix(t.Context(), parseUUID(testWorkspaceID)) + "-" + strconv.Itoa(issueNumber(t, hiddenID)))
		if hiddenByIdent.Code != http.StatusNotFound {
			t.Errorf("PatchPluginIssue by identifier as outsider: want 404, got %d %s", hiddenByIdent.Code, hiddenByIdent.Body.String())
		}
		if after := issueRowSnapshot(t, hiddenID); after != before {
			t.Errorf("a plugin patch of the outsider changed the hidden issue:\nbefore: %s\nafter:  %s", before, after)
		}
	})
}
