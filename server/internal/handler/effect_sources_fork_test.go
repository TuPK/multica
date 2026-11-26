package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"encoding/json"

	"github.com/multica-ai/multica/server/internal/connectedapps"
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

// TestDeclareFromCLIAnswersWithTheDecision: a CLI declare is a write, but the declare
// route answers with the policy's decision itself. A shadow task's CLI declare of a send
// gets 200 with decision block, so the CLI exits 4. The CLI gate must not refuse it
// with 403 first: the CLI reads 403 as the held code 3, which tells the agent to wait.
func TestDeclareFromCLIAnswersWithTheDecision(t *testing.T) {
	f := effectsOnFixture(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id = $1`, f.taskID)
	})
	if _, err := testPool.Exec(context.Background(),
		`UPDATE agent_task_queue SET context = '{"durable_kind":"shadow"}'::jsonb WHERE id = $1`, f.taskID); err != nil {
		t.Fatalf("mark shadow: %v", err)
	}

	chain := middleware.ClientMetadata(f.h.EffectCLIMiddleware(http.HandlerFunc(f.h.DeclareEffect)))
	req := effectAPIRequest(t, http.MethodPost, "/api/effects/declare", testUserID, f.taskID, map[string]any{
		"class": connectedapps.EffectSend, "reversibility": connectedapps.ReversibilityIrreversible,
		"target": "smtp:customer@example.com", "summary": "test",
	})
	req.Header.Set("X-Client-Platform", "cli")
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("CLI declare of a shadow send: %d %s, want 200 with the decision", w.Code, w.Body.String())
	}
	var out DeclareEffectResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Decision != connectedapps.PolicyBlock {
		t.Fatalf("decision %q, want block", out.Decision)
	}
	// The declare records its own event, so the CLI gate must not log a second one.
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND source='cli'`, f.taskID); n != 0 {
		t.Fatalf("CLI gate logged %d cli events for a declare, want 0", n)
	}
}
