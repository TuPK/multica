package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The shadow run routes (durable spec 9.4 and 9.5): a person sees the estimate, and a start over
// the budget the seat has left needs the person's confirmation.

// shadowSeatWithHistory is a seat on a squad with a one-USD monthly budget, and one completed task
// costing one USD, so a 20-pair run at rate 1 estimates 20 USD against 1 USD left.
func shadowSeatWithHistory(t *testing.T) (digestEnv, string) {
	t.Helper()
	e := newDigestEnv(t)
	squad := e.squad(t, "shadow squad")
	if _, err := testPool.Exec(context.Background(), `UPDATE agent SET home_squad_id = $2 WHERE id = $1`, e.AgentID, squad); err != nil {
		t.Fatal(err)
	}
	dbfx.Insert(t, "budget", testutil.Cols{
		"workspace_id": e.WorkspaceID, "scope_type": "squad", "scope_id": squad,
		"period": "month", "amount_micros": 1_000_000, "created_by": testUserID,
	})
	task := dbfx.Task(t, e.AgentID, testutil.Cols{"status": "completed", "completed_at": testutil.Raw("now() - interval '1 day'")})
	dbfx.Insert(t, "task_usage", testutil.Cols{
		"task_id": task, "provider": "anthropic", "model": "claude-sonnet-4.6",
		"cost_usd_ticks": int64(10_000_000_000), "updated_at": testutil.Raw("now()"),
	})
	candidate := dbfx.Runtime(t, "shadow candidate", testutil.Cols{"workspace_id": e.WorkspaceID})
	return e, candidate
}

func TestShadowRun_EstimateShowsTheWarning(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e, _ := shadowSeatWithHistory(t)

	w := httptest.NewRecorder()
	req := seatRequest("GET", "/api/agents/"+e.AgentID+"/shadow/estimate?sample_rate=1&target_pairs=20", e.WorkspaceID, nil, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.GetShadowEstimate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("estimate: status %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"estimate_micros":20000000`) || !strings.Contains(body, `"warn":true`) || !strings.Contains(body, `"remaining_micros":1000000`) {
		t.Fatalf("estimate body = %s, want 20000000 micros, a warning, and 1000000 left", body)
	}
}

// A start over the budget is refused with shadow_cost_confirmation_required and starts nothing;
// with confirm_cost it starts (201).
func TestShadowRun_StartNeedsConfirmationOverBudget(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e, candidate := shadowSeatWithHistory(t)
	path := "/api/agents/" + e.AgentID + "/shadow"
	body := map[string]any{"candidate_runtime_id": candidate, "candidate_model": "gpt-candidate", "sample_rate": 1, "target_pairs": 20}

	w := httptest.NewRecorder()
	req := seatRequest("POST", path, e.WorkspaceID, body, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.StartShadowRun(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "shadow_cost_confirmation_required") {
		t.Fatalf("start without confirmation: status %d body %s, want 409 shadow_cost_confirmation_required", w.Code, w.Body.String())
	}
	var started int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM shadow_config WHERE agent_id = $1`, e.AgentID).Scan(&started); err != nil {
		t.Fatal(err)
	}
	if started != 0 {
		t.Fatalf("a refused start still wrote %d shadow run(s)", started)
	}

	body["confirm_cost"] = true
	w = httptest.NewRecorder()
	req = seatRequest("POST", path, e.WorkspaceID, body, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.StartShadowRun(w, req)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"state":"running"`) {
		t.Fatalf("start with confirmation: status %d body %s, want 201 running", w.Code, w.Body.String())
	}
}

// The start is refused while the seat has a running shadow run (409 shadow_already_running).
func TestShadowRun_SecondStartIsRefused(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e, candidate := shadowSeatWithHistory(t)
	path := "/api/agents/" + e.AgentID + "/shadow"
	body := map[string]any{"candidate_runtime_id": candidate, "candidate_model": "gpt-candidate", "sample_rate": 1, "target_pairs": 20, "confirm_cost": true}
	for i, want := range []int{http.StatusCreated, http.StatusConflict} {
		w := httptest.NewRecorder()
		req := seatRequest("POST", path, e.WorkspaceID, body, "")
		req = withURLParam(req, "id", e.AgentID)
		testHandler.StartShadowRun(w, req)
		if w.Code != want {
			t.Fatalf("start %d: status %d body %s, want %d", i+1, w.Code, w.Body.String(), want)
		}
	}
}

// The shadow routes answer 404 while durable_seats is off.
func TestShadowRun_NotFoundWhileFlagOff(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, false)
	e := newDigestEnv(t)
	w := httptest.NewRecorder()
	req := seatRequest("GET", "/api/agents/"+e.AgentID+"/shadow/estimate", e.WorkspaceID, nil, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.GetShadowEstimate(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("estimate with the flag off: status %d, want 404", w.Code)
	}
}
