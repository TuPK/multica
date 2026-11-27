package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
)

// The seat's shadow run list (durable spec 9.4) is for members: a run's cost and verdicts are the
// person's to read, so an agent caller is refused.

func TestSeatShadowRuns_NotFoundWhileFlagOff(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, false)
	e := newDigestEnv(t)

	w := httptest.NewRecorder()
	req := seatRequest("GET", "/api/agents/"+e.AgentID+"/shadow", e.WorkspaceID, nil, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.ListSeatShadowRuns(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ListSeatShadowRuns with the flag off: status %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestSeatShadowRuns_AgentCallerRefused(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e := newDigestEnv(t)

	w := httptest.NewRecorder()
	req := seatRequest("GET", "/api/agents/"+e.AgentID+"/shadow", e.WorkspaceID, nil, e.AgentID)
	req = withURLParam(req, "id", e.AgentID)
	testHandler.ListSeatShadowRuns(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("agent caller: status %d, want 403: %s", w.Code, w.Body.String())
	}
}

// A member sees the run a person started, newest first, with its report. A fresh run has no
// compared pairs yet, so its report is zero and it is still running.
func TestSeatShadowRuns_MemberSeesStartedRun(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e, candidate := shadowSeatWithHistory(t)
	path := "/api/agents/" + e.AgentID + "/shadow"

	start := httptest.NewRecorder()
	startReq := seatRequest("POST", path, e.WorkspaceID, map[string]any{
		"candidate_runtime_id": candidate, "candidate_model": "gpt-candidate",
		"sample_rate": 1, "target_pairs": 20, "confirm_cost": true,
	}, "")
	startReq = withURLParam(startReq, "id", e.AgentID)
	testHandler.StartShadowRun(start, startReq)
	if start.Code != http.StatusCreated {
		t.Fatalf("start: status %d: %s", start.Code, start.Body.String())
	}

	w := httptest.NewRecorder()
	req := seatRequest("GET", path, e.WorkspaceID, nil, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.ListSeatShadowRuns(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: status %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"runs":[`, `"state":"running"`, `"purpose":"model_trial"`, `"report":{"pairs":0`} {
		if !strings.Contains(body, want) {
			t.Fatalf("list body = %s, want it to contain %s", body, want)
		}
	}
}
