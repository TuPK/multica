package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	sdk "github.com/multica-ai/multica/server/pkg/composio"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// withEffectGate wires the effect gate of the connected apps gateway (durable spec
// 4.4) with durable_effects set to on.
func withEffectGate(on bool) func(*connectedapps.Config) {
	return func(cfg *connectedapps.Config) {
		provider := featureflag.NewStaticProvider()
		provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: on})
		cfg.Effects = &service.EffectService{Queries: testHandler.Queries, Flags: featureflag.NewService(provider)}
	}
}

// TestConnectedAppsGatewayLogsEachCall: every call through the gateway writes one
// effect event with its outcome. At L1 a read runs, a send waits for approval and is
// not sent, and a shadow task's delete is blocked.
func TestConnectedAppsGatewayLogsEachCall(t *testing.T) {
	f := newAppsFixture(t, withEffectGate(true))
	f.executionAccount(t, "personal")
	if w := f.putAgentApps(t, testUserID, []map[string]string{{"toolkit_slug": "gmail", "account_scope": "personal", "level": "write"}}); w.Code != http.StatusOK {
		t.Fatalf("attach app: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id = $1`, f.taskID)
	})
	token := f.token(t)

	if _, text, isErr := f.gateway(t, token, "apps_run_tool", map[string]any{"tool": "GMAIL_FETCH_EMAILS", "arguments": map[string]any{"user_id": "me"}}); isErr {
		t.Fatalf("read call failed: %s", text)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND source='connected_app' AND status='executed'`, f.taskID); n != 1 {
		t.Fatalf("read call: %d executed events, want 1", n)
	}

	if _, text, isErr := f.gateway(t, token, "apps_run_tool", map[string]any{"tool": "GMAIL_SEND_EMAIL", "arguments": map[string]any{"user_id": "me"}}); !isErr || !strings.Contains(text, "approval") {
		t.Fatalf("send at L1 should wait for approval, got %q (error %v)", text, isErr)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Fatalf("provider calls after rehearsed send = %d, want 1 (the read only)", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND decision='rehearse' AND status='awaiting'`, f.taskID); n != 1 {
		t.Fatalf("rehearsed send: %d awaiting events, want 1", n)
	}

	if _, err := testPool.Exec(context.Background(),
		`UPDATE agent_task_queue SET context = '{"durable_kind":"shadow"}'::jsonb WHERE id = $1`, f.taskID); err != nil {
		t.Fatalf("mark task shadow: %v", err)
	}
	if _, text, isErr := f.gateway(t, token, "apps_run_tool", map[string]any{"tool": "GMAIL_DELETE_MESSAGE", "arguments": map[string]any{"user_id": "me"}}); !isErr {
		t.Fatalf("shadow delete ran: %s", text)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Fatalf("provider calls after shadow delete = %d, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND decision='block' AND status='blocked'`, f.taskID); n != 1 {
		t.Fatalf("shadow delete: %d blocked events, want 1", n)
	}
}

func TestApprovedActionRechecksEffectPolicy(t *testing.T) {
	f := newAppsFixture(t, withEffectGate(true))
	f.executionAccount(t, "personal")
	if w := f.putAgentApps(t, testUserID, []map[string]string{{"toolkit_slug": "gmail", "account_scope": "personal", "level": "confirm_writes"}}); w.Code != http.StatusOK {
		t.Fatalf("attach app: %d %s", w.Code, w.Body.String())
	}
	request := f.legacyRequest(t, "personal", "gmail", "GMAIL_SEND_EMAIL", "send", map[string]any{"to": "x@example.com"}, "")
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id=$1`, f.taskID) })
	if _, err := testPool.Exec(context.Background(), `UPDATE agent_task_queue SET context = '{"durable_kind":"shadow"}'::jsonb WHERE id=$1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	f.approve(t, request.ID, nil)
	if n := len(f.sdk.executions()); n != 0 {
		t.Errorf("blocked approved action provider calls = %d, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND task_id=$2 AND target='gmail/GMAIL_SEND_EMAIL' AND decision='block' AND status='blocked'`, testWorkspaceID, f.taskID); n != 1 {
		t.Errorf("blocked effect events = %d, want 1", n)
	}
}

func TestApprovedActionRecordsEffectAndResult(t *testing.T) {
	old := appsFakeGmailTools
	appsFakeGmailTools = append(append([]sdk.Tool{}, old...), sdk.Tool{Slug: "GMAIL_CREATE_DRAFT", Name: "Create draft", Toolkit: sdk.Toolkit{Slug: "gmail"}})
	t.Cleanup(func() { appsFakeGmailTools = old })
	f := newAppsFixture(t, withEffectGate(true))
	f.executionAccount(t, "personal")
	f.sdk.executeData = map[string]any{"id": "draft-result-123"}
	if w := f.putAgentApps(t, testUserID, []map[string]string{{"toolkit_slug": "gmail", "account_scope": "personal", "level": "confirm_writes"}}); w.Code != http.StatusOK {
		t.Fatalf("attach app: %d %s", w.Code, w.Body.String())
	}
	request := f.legacyRequest(t, "personal", "gmail", "GMAIL_CREATE_DRAFT", "write", map[string]any{"body": "draft"}, "")
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id=$1`, f.taskID) })
	f.approve(t, request.ID, nil)
	if n := len(f.sdk.executions()); n != 1 {
		t.Errorf("approved action provider calls = %d, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND task_id=$2 AND target='gmail/GMAIL_CREATE_DRAFT' AND status='executed'`, testWorkspaceID, f.taskID); n != 1 {
		t.Errorf("executed effect events = %d, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id=$1 AND task_id=$2 AND target='gmail/GMAIL_CREATE_DRAFT' AND undo->'args'->>'draft_id'='draft-result-123'`, testWorkspaceID, f.taskID); n != 1 {
		t.Errorf("events with undo resolved from result = %d, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event e WHERE workspace_id=$1 AND task_id=$2 AND target='gmail/GMAIL_CREATE_DRAFT' AND to_jsonb(e)->'result'->>'id'='draft-result-123'`, testWorkspaceID, f.taskID); n != 1 {
		t.Errorf("events containing provider result = %d, want 1", n)
	}
}
