package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
)

// seatRequest is a request on a seat as the given caller. An agent caller sends the headers
// the auth middleware stamps for a task token.
func seatRequest(method, path, workspaceID string, body any, agentID string) *http.Request {
	req := newRequestAs(testUserID, method, path, body)
	req.Header.Set("X-Workspace-ID", workspaceID)
	if agentID != "" {
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
	}
	return req
}

func seatEntryBody(text string) map[string]any {
	return map[string]any{"entries": []map[string]any{{
		"kind": "lesson", "text": text,
		"evidence": []map[string]string{{"type": "outcome", "ref": uuid.NewString()}},
	}}}
}

// The dossier routes answer 404 while durable_seats is off.
func TestSeatDossier_NotFoundWhileFlagOff(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, false)
	e := newDigestEnv(t)

	w := httptest.NewRecorder()
	req := seatRequest("GET", "/api/agents/"+e.AgentID+"/dossier", e.WorkspaceID, nil, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.ListSeatDossier(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ListSeatDossier with the flag off: status %d, want 404: %s", w.Code, w.Body.String())
	}
}

// A member and the seat's own agent write entries; each entry records its origin. Only a
// member retires a member's entry, and an agent is refused it.
func TestSeatDossier_OriginAndRetireRules(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e := newDigestEnv(t)
	path := "/api/agents/" + e.AgentID + "/dossier"

	// Member writes.
	w := httptest.NewRecorder()
	req := seatRequest("POST", path, e.WorkspaceID, seatEntryBody("member note"), "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.AddSeatDossierEntries(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("member add: status %d, want 201: %s", w.Code, w.Body.String())
	}
	var memberOut struct {
		Entries []SeatDossierEntryResponse `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &memberOut); err != nil || len(memberOut.Entries) != 1 {
		t.Fatalf("member add response: %v %s", err, w.Body.String())
	}
	if got := memberOut.Entries[0].Origin; got != "member" {
		t.Errorf("member entry origin = %q, want member", got)
	}
	memberEntry := memberOut.Entries[0].ID

	// The seat's agent writes.
	w = httptest.NewRecorder()
	req = seatRequest("POST", path, e.WorkspaceID, seatEntryBody("agent note"), e.AgentID)
	req = withURLParam(req, "id", e.AgentID)
	testHandler.AddSeatDossierEntries(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("agent add: status %d, want 201: %s", w.Code, w.Body.String())
	}

	// The agent may not retire the member's entry.
	w = httptest.NewRecorder()
	req = seatRequest("POST", path+"/"+memberEntry+"/retire", e.WorkspaceID, map[string]any{"reason": "stale"}, e.AgentID)
	req = withURLParams(req, "id", e.AgentID, "entryId", memberEntry)
	testHandler.RetireSeatDossierEntry(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("agent retires a member entry: status %d, want 403: %s", w.Code, w.Body.String())
	}

	// The member retires it.
	w = httptest.NewRecorder()
	req = seatRequest("POST", path+"/"+memberEntry+"/retire", e.WorkspaceID, map[string]any{"reason": "stale"}, "")
	req = withURLParams(req, "id", e.AgentID, "entryId", memberEntry)
	testHandler.RetireSeatDossierEntry(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("member retires: status %d, want 200: %s", w.Code, w.Body.String())
	}

	// The active list now holds only the agent's entry; the all list holds both.
	for status, want := range map[string]int{"active": 1, "all": 2} {
		w = httptest.NewRecorder()
		req = seatRequest("GET", path+"?status="+status, e.WorkspaceID, nil, "")
		req = withURLParam(req, "id", e.AgentID)
		testHandler.ListSeatDossier(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list %s: status %d: %s", status, w.Code, w.Body.String())
		}
		var out struct {
			Entries []SeatDossierEntryResponse `json:"entries"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("list %s decode: %v", status, err)
		}
		if len(out.Entries) != want {
			t.Errorf("list %s: %d entries, want %d", status, len(out.Entries), want)
		}
	}
}
