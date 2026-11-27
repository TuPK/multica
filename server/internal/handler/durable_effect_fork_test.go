package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	sdk "github.com/multica-ai/multica/server/pkg/composio"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// effectAPIRequest is a request as the authenticated group would pass it on: the
// workspace header, and the acting user and task set the way auth sets them.
func effectAPIRequest(t *testing.T, method, path, userID, taskID string, body any) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(encodeEffectBody(t, body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	req.Header.Set("X-User-ID", userID)
	if taskID != "" {
		req.Header.Set("X-Task-ID", taskID)
	}
	return req
}

// effectUndoRequest is the undo call for one effect id, with the route parameter set.
func effectUndoRequest(t *testing.T, id, userID string) *http.Request {
	t.Helper()
	req := effectAPIRequest(t, http.MethodPost, "/api/effects/"+id+"/undo", userID, "", nil)
	return testutil.WithURLParams(req, "id", id)
}

func encodeEffectBody(t *testing.T, body any) string {
	t.Helper()
	if body == nil {
		return ""
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	return string(raw)
}

// effectsOnFixture is the connected apps fixture with the effect gate on, the gmail
// personal account connected for the test user, and a run that is an L1 agent's.
func effectsOnFixture(t *testing.T) *appsFixture {
	t.Helper()
	f := newAppsFixture(t, withEffectGate(true))
	f.h.durable.Effects = &service.EffectService{Queries: testHandler.Queries, Flags: effectFlagsOn()}
	f.executionAccount(t, "personal")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE workspace_id = $1 AND target LIKE 'gmail/%'`, testWorkspaceID)
	})
	return f
}

// effectFlagsOn is the feature flag service with durable_effects on.
func effectFlagsOn() *featureflag.Service {
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	return featureflag.NewService(provider)
}

// TestDeclareEffectDecisions: a declared action gets the policy's decision for the
// agent's level and task. An L1 agent's send rehearses, a read runs, and a shadow
// task's send is blocked (durable spec 4.3, 4.5).
func TestDeclareEffectDecisions(t *testing.T) {
	f := effectsOnFixture(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id = $1`, f.taskID)
	})
	declare := func(class, target string) DeclareEffectResponse {
		t.Helper()
		w := httptest.NewRecorder()
		f.h.DeclareEffect(w, effectAPIRequest(t, http.MethodPost, "/api/effects/declare", testUserID, f.taskID, map[string]any{
			"class": class, "target": target, "summary": "test",
		}))
		if w.Code != http.StatusOK {
			t.Fatalf("declare %s: %d %s", class, w.Code, w.Body.String())
		}
		var out DeclareEffectResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	if got := declare(connectedapps.EffectRead, "smtp:customer@example.com"); got.Decision != connectedapps.PolicyAllow {
		t.Fatalf("read decision %q, want allow", got.Decision)
	}
	if got := declare(connectedapps.EffectSend, "smtp:customer@example.com"); got.Decision != connectedapps.PolicyRehearse || got.Status != "declared" {
		t.Fatalf("send at L1 = %+v, want rehearse with status declared", got)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND source='declared' AND status='declared'`, f.taskID); n != 2 {
		t.Fatalf("declared events = %d, want 2", n)
	}

	if _, err := testPool.Exec(context.Background(),
		`UPDATE agent_task_queue SET context = '{"durable_kind":"shadow"}'::jsonb WHERE id = $1`, f.taskID); err != nil {
		t.Fatalf("mark shadow: %v", err)
	}
	if got := declare(connectedapps.EffectSend, "smtp:customer@example.com"); got.Decision != connectedapps.PolicyBlock {
		t.Fatalf("shadow send = %+v, want block", got)
	}
}

// TestDeclareEffectValidation: the class and the target are required, and a
// reversibility must be one of the three values.
func TestDeclareEffectValidation(t *testing.T) {
	f := effectsOnFixture(t)
	cases := []map[string]any{
		{"class": "bogus", "target": "x"},
		{"class": connectedapps.EffectSend, "target": ""},
		{"class": connectedapps.EffectSend, "target": "x", "reversibility": "maybe"},
	}
	for _, body := range cases {
		w := httptest.NewRecorder()
		f.h.DeclareEffect(w, effectAPIRequest(t, http.MethodPost, "/api/effects/declare", testUserID, f.taskID, body))
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("body %v: status %d, want 422", body, w.Code)
		}
	}
}

// seedGmailDraftEffect writes an executed effect whose undo deletes the draft. The
// test runs the forward call's effect directly, since the undo is what is under test.
func seedGmailDraftEffect(t *testing.T, f *appsFixture, undoUntil time.Time, undo bool) string {
	t.Helper()
	undoJSON := ""
	if undo {
		undoJSON = `{"tool":"GMAIL_DELETE_DRAFT","args":{"draft_id":"d-1"}}`
	}
	id := dbfx.Insert(t, "effect_event", testutil.Cols{
		"workspace_id": testWorkspaceID, "task_id": f.taskID, "agent_id": f.agentID, "member_id": testUserID,
		"source": "connected_app", "target": "gmail/GMAIL_CREATE_DRAFT", "effect_class": "write",
		"reversibility": "reversible", "decision": "allow", "status": "executed", "summary": "draft",
		"undo":       testutil.Raw(quoteSQLJSON(undoJSON)),
		"undo_until": undoUntil,
	})
	return id
}

// quoteSQLJSON is a JSON literal for the undo column, or NULL when there is no undo.
func quoteSQLJSON(s string) string {
	if s == "" {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'::jsonb"
}

func withGmailDraftTools(t *testing.T) {
	t.Helper()
	old := appsFakeGmailTools
	appsFakeGmailTools = append(append([]sdk.Tool{}, old...),
		sdk.Tool{Slug: "GMAIL_DELETE_DRAFT", Name: "Delete draft", Toolkit: sdk.Toolkit{Slug: "gmail"}})
	t.Cleanup(func() { appsFakeGmailTools = old })
}

// TestUndoCallsInverseTool: undoing an executed draft calls the inverse tool with the
// draft id from the forward result, and the event becomes undone.
func TestUndoCallsInverseTool(t *testing.T) {
	withGmailDraftTools(t)
	f := effectsOnFixture(t)
	id := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)

	// The inverse call goes through the gateway as the member who acted.
	spec := connectedapps.UndoSpec{Tool: "GMAIL_DELETE_DRAFT", Args: map[string]any{"draft_id": "d-1"}}
	data, err := f.h.ConnectedApps.ExecuteUndo(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), "gmail/GMAIL_CREATE_DRAFT", spec)
	if err != nil {
		t.Fatalf("ExecuteUndo: %v", err)
	}
	if data["tool"] != "GMAIL_DELETE_DRAFT" {
		t.Fatalf("inverse tool ran %v, want GMAIL_DELETE_DRAFT", data["tool"])
	}
	if execs := f.sdk.executions(); len(execs) == 0 || execs[len(execs)-1].Arguments["draft_id"] != "d-1" {
		t.Fatalf("inverse call arguments: %+v", execs)
	}

	w := httptest.NewRecorder()
	f.h.UndoEffect(w, effectUndoRequest(t, id, testUserID))
	if w.Code != http.StatusOK {
		t.Fatalf("undo: %d %s", w.Code, w.Body.String())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='undone'`, id); n != 1 {
		t.Fatalf("original event not undone")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND summary = $2`, testWorkspaceID, "Hoàn tác "+id); n != 1 {
		t.Fatalf("undo recorded %d events, want 1", n)
	}
}

// TestUndoAfterWindow409: an effect whose undo window has closed cannot be undone.
func TestUndoAfterWindow409(t *testing.T) {
	f := effectsOnFixture(t)
	id := seedGmailDraftEffect(t, f, time.Now().Add(-time.Hour), true)

	w := httptest.NewRecorder()
	f.h.UndoEffect(w, effectUndoRequest(t, id, testUserID))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "undo_window_closed") {
		t.Fatalf("undo after window: %d %s, want 409 undo_window_closed", w.Code, w.Body.String())
	}
}

// TestUndoNotUndoable422: an effect with no undo (a write the platform cannot take
// back) answers 422.
func TestUndoNotUndoable422(t *testing.T) {
	f := effectsOnFixture(t)
	id := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), false)

	w := httptest.NewRecorder()
	f.h.UndoEffect(w, effectUndoRequest(t, id, testUserID))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "not_undoable") {
		t.Fatalf("undo without undo: %d %s, want 422 not_undoable", w.Code, w.Body.String())
	}
}

// TestListEffectsFilters: the list is the workspace's events, newest first, and a
// filter by class narrows it.
func TestListEffectsFilters(t *testing.T) {
	f := effectsOnFixture(t)
	seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)

	w := httptest.NewRecorder()
	req := effectAPIRequest(t, http.MethodGet, "/api/effects?effect_class=write&limit=5", testUserID, "", nil)
	f.h.ListEffects(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Effects []EffectResponse `json:"effects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Effects) == 0 || !out.Effects[0].UndoAvailable {
		t.Fatalf("list = %+v, want the seeded write with undo available", out.Effects)
	}
	for _, e := range out.Effects {
		if e.EffectClass != "write" {
			t.Fatalf("filter leaked class %q", e.EffectClass)
		}
	}
}
