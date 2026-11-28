package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
)

func TestPluginActionWritesLogOneEvent(t *testing.T) {
	installationID := installPluginForAction(t, []string{"issues:read", "issues:write", "comments:write", "storage:user"})
	issueID := createTestIssue(t, "Effect log action", "todo", "none")
	h := *testHandler
	h.durable.Effects = &service.EffectService{Queries: h.Queries, Flags: effectFlagsOn()}

	cases := []struct {
		name, method, target string
		body                 any
		params               map[string]string
		handle               http.HandlerFunc
		status               int
	}{
		{"patch issue", http.MethodPatch, "PATCH issue", map[string]any{"title": "Updated"}, map[string]string{"issue_ref": issueID}, h.PatchPluginIssue, http.StatusOK},
		{"create comment", http.MethodPost, "POST issue comment", map[string]any{"content": "Effect test"}, map[string]string{"issue_ref": issueID}, h.CreatePluginComment, http.StatusCreated},
		{"put storage", http.MethodPut, "PUT plugin storage", map[string]any{"value": "test"}, map[string]string{"scope": "user", "key": "effect-test"}, h.PutPluginStorage, http.StatusNoContent},
		{"delete storage", http.MethodDelete, "DELETE plugin storage", nil, map[string]string{"scope": "user", "key": "effect-test"}, h.DeletePluginStorage, http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND source='plugin_action' AND target=$2`, testWorkspaceID, tc.target)
			rec := httptest.NewRecorder()
			tc.handle(rec, pluginActionRequest(tc.method, "/v1/effect-test", installationID, tc.body, tc.params))
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			after := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND source='plugin_action' AND target=$2`, testWorkspaceID, tc.target)
			if after != before+1 {
				t.Fatalf("event count before=%d after=%d, want exactly one new event", before, after)
			}
			var id string
			var class, status string
			if err := testPool.QueryRow(context.Background(), `SELECT id, effect_class, status FROM effect_event WHERE workspace_id=$1 AND source='plugin_action' AND target=$2 ORDER BY created_at DESC LIMIT 1`, testWorkspaceID, tc.target).Scan(&id, &class, &status); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE workspace_id=$1 AND id=$2`, testWorkspaceID, id)
			})
			if class != "write_internal" || status != "executed" {
				t.Fatalf("event class=%q status=%q, want write_internal/executed", class, status)
			}
		})
	}
}
