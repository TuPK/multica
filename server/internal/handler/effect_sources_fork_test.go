package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestEffectCLIMiddlewareLogsMutations: a mutating request from the CLI writes one
// cli effect with its outcome; a read and a request from another client write none.
func TestEffectCLIMiddlewareLogsMutations(t *testing.T) {
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	h := *testHandler
	h.Effects = &service.EffectService{Queries: testHandler.Queries, Flags: featureflag.NewService(provider)}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE workspace_id = $1 AND source = 'cli' AND target LIKE 'POST /api/effect-test%'`, testWorkspaceID)
	})

	status := http.StatusCreated
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
	handler := middleware.ClientMetadata(h.EffectCLIMiddleware(next))

	do := func(method, client string) int {
		req := httptest.NewRequest(method, "/api/effect-test", nil)
		req.Header.Set("X-Workspace-ID", testWorkspaceID)
		req.Header.Set("X-Client-Platform", client)
		req.Header.Set("X-User-ID", testUserID)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(http.MethodPost, "cli"); code != http.StatusCreated {
		t.Fatalf("CLI POST status %d, want 201", code)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND source='cli' AND status='executed' AND target='POST /api/effect-test'`, testWorkspaceID); n != 1 {
		t.Fatalf("CLI POST: %d executed cli events, want 1", n)
	}

	status = http.StatusBadRequest
	do(http.MethodPost, "cli")
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND source='cli' AND status='failed' AND target='POST /api/effect-test'`, testWorkspaceID); n != 1 {
		t.Fatalf("CLI POST answered 400: %d failed cli events, want 1", n)
	}

	status = http.StatusOK
	do(http.MethodGet, "cli")
	do(http.MethodPost, "web")
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND source='cli' AND target LIKE 'GET %'`, testWorkspaceID); n != 0 {
		t.Fatalf("CLI GET wrote %d events, want 0", n)
	}
}
