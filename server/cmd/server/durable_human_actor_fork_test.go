package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/realtime"
	dbfx "github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestDurableHumanOnlyRoutesRejectMachineCredentials runs the real Auth -> router -> handler
// chain for review findings 9 and 10: both task_token (mat_) and cloud_pat (mcn_) are rejected
// with 403 on human-only effect/intent, principal, and squad autonomy envelope routes, while
// POST /api/effects/declare and GET routes remain accessible to task_token and human JWT reaches
// the handlers.
func TestDurableHumanOnlyRoutesRejectMachineCredentials(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	f := dbfx.New(testPool, testWorkspaceID, testUserID)
	f.WorkspaceID = f.Workspace(t, "Durable actor guards", "durable-guards-"+uuid.NewString())
	f.Member(t, f.WorkspaceID, f.UserID, "owner")

	runtimeID := f.Runtime(t, "Durable actor guards runtime")
	agentID := f.Agent(t, "Durable actor guards agent", runtimeID)
	squadID := f.Squad(t, "Durable actor guards squad", agentID)
	taskID := f.Task(t, agentID, dbfx.Cols{"runtime_id": runtimeID})
	f.Cleanup(t, `DELETE FROM squad_autonomy_envelope WHERE squad_id = $1`, squadID)
	f.Cleanup(t, `DELETE FROM principal_trait WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM principal_profile WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1`, f.WorkspaceID)

	taskTok := "mat_" + uuid.NewString()
	f.InsertNoID(t, "task_token", dbfx.Cols{
		"token_hash": auth.HashToken(taskTok), "task_id": taskID, "agent_id": agentID,
		"workspace_id": f.WorkspaceID, "user_id": f.UserID,
		"expires_at": dbfx.Raw("now() + interval '1 hour'"),
	}, "token_hash = $1", auth.HashToken(taskTok))

	cloudTok := "mcn_" + uuid.NewString()
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Token string `json:"token"`
		}
		if r.URL.Path != "/api/v1/pat/verify" || json.NewDecoder(r.Body).Decode(&request) != nil || request.Token != cloudTok {
			http.Error(w, "invalid verification request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "owner_id": f.UserID})
	}))
	t.Cleanup(fleet.Close)
	t.Setenv("MULTICA_CLOUD_URL", fleet.URL)

	flags := featureflag.NewStaticProvider()
	flags.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	flags.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	flags.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	router, _ := NewRouterWithOptions(testPool, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil,
		RouterOptions{FeatureFlags: featureflag.NewService(flags)})

	call := func(t *testing.T, method, path, token string, body any) *dbfx.Response {
		t.Helper()
		req := dbfx.JSONRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Workspace-ID", f.WorkspaceID)
		req.Header.Set("X-Actor-Source", "member")
		return dbfx.Call(t, router.ServeHTTP, req)
	}

	dummyID := uuid.NewString()
	humanOnly := []struct {
		method string
		path   string
		body   any
	}{
		// Finding 9: /api/effects and /api/intents decision/undo/policy routes
		{http.MethodPost, "/api/effects/" + dummyID + "/undo", nil},
		{http.MethodPut, "/api/effects/policy", map[string]any{"autonomy_level": 1, "effect_class": "write", "reversibility": "compensable", "decision": "block"}},
		{http.MethodPost, "/api/intents/approve-batch", map[string]any{"ids": []string{dummyID}}},
		{http.MethodPost, "/api/intents/" + dummyID + "/approve", map[string]any{}},
		{http.MethodPost, "/api/intents/" + dummyID + "/reject", map[string]any{"note": "no"}},
		// Finding 10: /api/principal/* person-only routes & squad autonomy envelope writes
		{http.MethodPut, "/api/principal/profile", map[string]any{"consent_text": "Tôi đồng ý"}},
		{http.MethodDelete, "/api/principal/profile", nil},
		{http.MethodPatch, "/api/principal/profile/traits/" + dummyID, map[string]any{"statement": "x"}},
		{http.MethodPut, "/api/principal/delegations", map[string]any{"decision_kind": "general", "min_confidence": 0.9}},
		{http.MethodDelete, "/api/principal/delegations?decision_kind=general", nil},
		{http.MethodPost, "/api/principal/decisions/" + dummyID + "/revert", nil},
		{http.MethodGet, "/api/principal/interview", nil},
		{http.MethodPost, "/api/principal/interview/answers", map[string]any{"question": 0, "answer": "a"}},
		{http.MethodPut, "/api/squads/" + squadID + "/autonomy/envelope", map[string]any{"floor_level": 0, "ceiling_level": 2, "reason": "human gate"}},
		{http.MethodDelete, "/api/squads/" + squadID + "/autonomy/envelope", nil},
	}

	for credName, tok := range map[string]string{"task_token": taskTok, "cloud_pat": cloudTok} {
		for _, tc := range humanOnly {
			t.Run(credName+"_"+tc.method+"_"+tc.path, func(t *testing.T) {
				call(t, tc.method, tc.path, tok, tc.body).Want(http.StatusForbidden)
			})
		}
	}

	// Agent task_token retains access to declare and GET endpoints on /api/effects and /api/intents.
	call(t, http.MethodGet, "/api/effects", taskTok, nil).Want(http.StatusOK)
	call(t, http.MethodGet, "/api/effects/policy", taskTok, nil).Want(http.StatusOK)
	call(t, http.MethodGet, "/api/intents", taskTok, nil).Want(http.StatusOK)
	call(t, http.MethodPost, "/api/effects/declare", taskTok, map[string]any{
		"class": "read", "target": "smtp:customer@example.com", "summary": "agent check",
	}).Want(http.StatusOK)
	f.Cleanup(t, `DELETE FROM effect_event WHERE workspace_id = $1`, f.WorkspaceID)

	// Control: human JWT passes RequireHumanActor and succeeds on profile & envelope writes.
	call(t, http.MethodPut, "/api/principal/profile", testToken, map[string]any{"consent_text": "Tôi đồng ý"}).Want(http.StatusOK)
	call(t, http.MethodGet, "/api/principal/interview", testToken, nil).Want(http.StatusOK)
	call(t, http.MethodPut, "/api/squads/"+squadID+"/autonomy/envelope", testToken,
		map[string]any{"floor_level": 0, "ceiling_level": 2, "reason": "owner sets envelope"}).Want(http.StatusOK)
	call(t, http.MethodDelete, "/api/squads/"+squadID+"/autonomy/envelope", testToken, nil).Want(http.StatusNoContent)
	call(t, http.MethodDelete, "/api/principal/profile", testToken, nil).Want(http.StatusNoContent)
}
