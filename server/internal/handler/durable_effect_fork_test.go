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

func TestUndoConcurrentClaimsBeforeCallingTool(t *testing.T) {
	withGmailDraftTools(t)
	f := effectsOnFixture(t)
	id := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	f.sdk.beforeExecuteTool = func() { entered <- struct{}{}; <-release }
	reqs := []*http.Request{effectUndoRequest(t, id, testUserID), effectUndoRequest(t, id, testUserID)}
	responses := make(chan int, 2)
	run := func(req *http.Request) { w := httptest.NewRecorder(); f.h.UndoEffect(w, req); responses <- w.Code }
	go run(reqs[0])
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first undo did not reach the provider")
	}
	go run(reqs[1])
	// The second call must finish with a conflict while the first is still in the provider.
	var second int
	select {
	case second = <-responses:
	case <-entered:
	case <-time.After(5 * time.Second):
	}
	close(release)
	if second == 0 {
		second = <-responses
	}
	first := <-responses
	if !((first == http.StatusOK && second == http.StatusConflict) || (second == http.StatusOK && first == http.StatusConflict)) {
		t.Errorf("undo statuses = %d, %d; want one 200 and one 409", first, second)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Errorf("provider calls = %d, want 1", n)
	}
}

func TestUndoFlagOffDoesNotCallProvider(t *testing.T) {
	withGmailDraftTools(t)
	f := effectsOnFixture(t)
	id := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)
	off := featureflag.NewStaticProvider()
	off.Set(featureflags.DurableEffects, featureflag.Rule{Default: false})
	f.h.durable.Effects = &service.EffectService{Queries: testHandler.Queries, Flags: featureflag.NewService(off)}
	f.h.FeatureFlags = featureflag.NewService(off)
	w := httptest.NewRecorder()
	f.h.UndoEffect(w, effectUndoRequest(t, id, testUserID))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("undo status = %d, want 503: %s", w.Code, w.Body.String())
	}
	if n := len(f.sdk.executions()); n != 0 {
		t.Errorf("provider calls = %d, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='executed'`, id); n != 1 {
		t.Error("effect did not remain executed")
	}
}

func TestUndoProviderFailureLeavesEffectExecuted(t *testing.T) {
	withGmailDraftTools(t)
	f := effectsOnFixture(t)
	id := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)
	f.sdk.toolError = "provider failed"
	w := httptest.NewRecorder()
	f.h.UndoEffect(w, effectUndoRequest(t, id, testUserID))
	if w.Code != http.StatusBadGateway {
		t.Errorf("undo status = %d, want 502", w.Code)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Errorf("provider calls = %d, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='executed'`, id); n != 1 {
		t.Error("effect did not remain executed")
	}
}

// TestUndoInternalRevisionKinds: v1 write_internal undo supports the three revision kinds
// (issue_description, agent_instruction, knowledge_draft); any other kind or a published
// knowledge link answers 422 "không hoàn tác được" (durable spec 4.6, plan B3).
func TestUndoInternalRevisionKinds(t *testing.T) {
	f := effectsOnFixture(t)
	ctx := context.Background()

	seedInternal := func(target string, undoJSON string) string {
		t.Helper()
		id := dbfx.Insert(t, "effect_event", testutil.Cols{
			"workspace_id": testWorkspaceID, "task_id": f.taskID, "agent_id": f.agentID, "member_id": testUserID,
			"source": "cli", "target": target, "effect_class": connectedapps.EffectWriteInternal,
			"reversibility": connectedapps.ReversibilityReversible, "decision": "allow", "status": "executed",
			"summary":    "internal edit",
			"undo":       testutil.Raw(quoteSQLJSON(undoJSON)),
			"undo_until": time.Now().Add(time.Hour),
		})
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE id = $1 OR summary = $2`, id, "Hoàn tác "+id)
		})
		return id
	}

	t.Run("issue_description restores prior revision", func(t *testing.T) {
		issueID := dbfx.Issue(t, "undo desc issue", testutil.Cols{"description": "after agent edit"})
		if _, err := testPool.Exec(ctx,
			`INSERT INTO issue_description_revision (issue_id, workspace_id, revision, content) VALUES ($1, $2, 1, 'before agent edit')`,
			issueID, testWorkspaceID); err != nil {
			t.Fatalf("seed description revision: %v", err)
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM issue_description_revision WHERE issue_id = $1`, issueID)
		})
		evID := seedInternal("issue/description", `{"kind":"issue_description","target_id":"`+issueID+`","revision":1}`)

		w := httptest.NewRecorder()
		f.h.UndoEffect(w, effectUndoRequest(t, evID, testUserID))
		if w.Code != http.StatusOK {
			t.Fatalf("undo issue_description: %d %s", w.Code, w.Body.String())
		}
		var desc string
		if err := testPool.QueryRow(ctx, `SELECT description FROM issue WHERE id = $1`, issueID).Scan(&desc); err != nil {
			t.Fatalf("read issue description: %v", err)
		}
		if desc != "before agent edit" {
			t.Fatalf("issue description = %q, want %q", desc, "before agent edit")
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='undone'`, evID); n != 1 {
			t.Fatal("effect event not marked undone")
		}
	})

	t.Run("agent_instruction restores prior revision", func(t *testing.T) {
		rtID := dbfx.Runtime(t, "undo instr runtime")
		agentID := dbfx.Agent(t, "undo instr agent", rtID, testutil.Cols{"instructions": "v2 agent instructions"})
		if _, err := testPool.Exec(ctx, `
			INSERT INTO agent_instruction_revision (workspace_id, agent_id, revision, instructions, source, actor_type, actor_id)
			VALUES ($1, $2, 1, 'v1 original instructions', 'human', 'member', $3)`,
			testWorkspaceID, agentID, testUserID); err != nil {
			t.Fatalf("seed instruction revision: %v", err)
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM agent_instruction_revision WHERE agent_id = $1`, agentID)
		})
		evID := seedInternal("agent/instructions", `{"kind":"agent_instruction","target_id":"`+agentID+`","revision":1}`)

		w := httptest.NewRecorder()
		f.h.UndoEffect(w, effectUndoRequest(t, evID, testUserID))
		if w.Code != http.StatusOK {
			t.Fatalf("undo agent_instruction: %d %s", w.Code, w.Body.String())
		}
		var instr string
		if err := testPool.QueryRow(ctx, `SELECT instructions FROM agent WHERE id = $1`, agentID).Scan(&instr); err != nil {
			t.Fatalf("read agent instructions: %v", err)
		}
		if instr != "v1 original instructions" {
			t.Fatalf("agent instructions = %q, want %q", instr, "v1 original instructions")
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='undone'`, evID); n != 1 {
			t.Fatal("effect event not marked undone")
		}
	})

	t.Run("knowledge_draft discards draft and refuses published", func(t *testing.T) {
		kbID := dbfx.Insert(t, "knowledge_base", testutil.Cols{
			"workspace_id": testWorkspaceID, "weknora_kb_id": "kb-undo-test", "key": "undo-" + f.taskID[:8], "name": "Undo KB",
		})
		draftID := dbfx.Insert(t, "knowledge_link", testutil.Cols{
			"workspace_id": testWorkspaceID, "knowledge_base_id": kbID, "weknora_knowledge_id": "",
			"source_type": "issue", "idempotency_key": "draft-" + f.taskID, "state": "draft",
			"created_by_type": "agent", "created_by_id": f.agentID,
		})
		pubID := dbfx.Insert(t, "knowledge_link", testutil.Cols{
			"workspace_id": testWorkspaceID, "knowledge_base_id": kbID, "weknora_knowledge_id": "",
			"source_type": "issue", "idempotency_key": "pub-" + f.taskID, "state": "published",
			"created_by_type": "agent", "created_by_id": f.agentID,
		})

		draftEv := seedInternal("knowledge/draft", `{"kind":"knowledge_draft","target_id":"`+draftID+`"}`)
		w := httptest.NewRecorder()
		f.h.UndoEffect(w, effectUndoRequest(t, draftEv, testUserID))
		if w.Code != http.StatusOK {
			t.Fatalf("undo knowledge_draft: %d %s", w.Code, w.Body.String())
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM knowledge_link WHERE id = $1`, draftID); n != 0 {
			t.Fatalf("draft knowledge_link still exists (%d)", n)
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='undone'`, draftEv); n != 1 {
			t.Fatal("draft effect event not marked undone")
		}

		pubEv := seedInternal("knowledge/draft", `{"kind":"knowledge_draft","target_id":"`+pubID+`"}`)
		w2 := httptest.NewRecorder()
		f.h.UndoEffect(w2, effectUndoRequest(t, pubEv, testUserID))
		if w2.Code != http.StatusUnprocessableEntity || !strings.Contains(w2.Body.String(), "không hoàn tác được") {
			t.Fatalf("undo published knowledge link: %d %s, want 422 không hoàn tác được", w2.Code, w2.Body.String())
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='executed'`, pubEv); n != 1 {
			t.Fatal("published effect event should stay executed")
		}
	})

	t.Run("unsupported kind answers 422 không hoàn tác được", func(t *testing.T) {
		for _, kind := range []string{"comment", "issue_status", "unknown_kind"} {
			evID := seedInternal("issue/"+kind, `{"kind":"`+kind+`","target_id":"`+f.taskID+`"}`)
			w := httptest.NewRecorder()
			f.h.UndoEffect(w, effectUndoRequest(t, evID, testUserID))
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "không hoàn tác được") {
				t.Fatalf("undo kind %q: %d %s, want 422 không hoàn tác được", kind, w.Code, w.Body.String())
			}
			if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status='executed'`, evID); n != 1 {
				t.Fatalf("kind %q event should stay executed", kind)
			}
		}
	})
}
