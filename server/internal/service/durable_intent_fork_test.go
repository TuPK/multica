package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

type fakeIntentExecutor struct {
	calls []connectedapps.CanonicalCall
	err   error
}

func (f *fakeIntentExecutor) ExecuteIntent(_ context.Context, _, _ pgtype.UUID, _ string, call connectedapps.CanonicalCall) (map[string]any, error) {
	f.calls = append(f.calls, call)
	if f.err != nil {
		return nil, f.err
	}
	return map[string]any{"ok": true}, nil
}

type intentServiceFixture struct {
	principalFixture
	ws      pgtype.UUID
	user    pgtype.UUID
	runtime string
	agent   string
	issue   string
	task    string
	effects *EffectService
	intents *IntentService
	exec    *fakeIntentExecutor
	now     time.Time
}

func newIntentServiceFixture(t *testing.T) *intentServiceFixture {
	t.Helper()
	f, _ := companyWorkspace(t)
	f.Cleanup(t, `DELETE FROM effect_intent WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM effect_event WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM comment WHERE workspace_id = $1`, f.WorkspaceID)

	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	flags := featureflag.NewService(provider)

	runtimeID := f.Runtime(t, "intent runtime")
	agentID := f.Agent(t, "intent agent", runtimeID)
	issueID := f.Issue(t, "intent issue")
	taskID := f.Task(t, agentID, testutil.Cols{"issue_id": issueID, "runtime_id": runtimeID, "status": "running"})

	fx := &intentServiceFixture{
		principalFixture: f,
		ws:               util.MustParseUUID(f.WorkspaceID),
		user:             util.MustParseUUID(f.UserID),
		runtime:          runtimeID,
		agent:            agentID,
		issue:            issueID,
		task:             taskID,
		exec:             &fakeIntentExecutor{},
		now:              time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC),
	}
	fx.effects = &EffectService{Queries: f.q, Flags: flags, Now: func() time.Time { return fx.now }}
	fx.intents = &IntentService{
		Queries:   f.q,
		TxStarter: f.Pool,
		Flags:     flags,
		Apps:      fx.exec,
		Tasks:     &TaskService{Queries: f.q, TxStarter: f.Pool, Bus: events.New()},
		Now:       func() time.Time { return fx.now },
	}
	return fx
}

func (fx *intentServiceFixture) rehearseEmail(t *testing.T, body string) (pgtype.UUID, connectedapps.CanonicalCall) {
	t.Helper()
	ctx := context.Background()
	req := connectedapps.EffectRequest{
		WS:            fx.ws,
		TaskID:        util.MustParseUUID(fx.task),
		AgentID:       util.MustParseUUID(fx.agent),
		MemberID:      fx.user,
		IssueID:       util.MustParseUUID(fx.issue),
		Source:        connectedapps.SourceConnectedApp,
		Target:        "gmail/GMAIL_SEND_EMAIL",
		Class:         connectedapps.EffectSend,
		Reversibility: connectedapps.ReversibilityIrreversible,
		Summary:       "Send quote",
		Prior:         connectedapps.PolicyAllow,
	}
	ticket, err := fx.effects.Gate(ctx, req)
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	call := connectedapps.CanonicalCall{
		Tool:      "GMAIL_SEND_EMAIL",
		AccountID: "acc-1",
		Args:      map[string]any{"to": "customer@example.com", "subject": "Quote", "body": body},
	}
	id, err := fx.intents.Rehearse(ctx, ticket.EventID, req, call, "customer requested the quote")
	if err != nil {
		t.Fatalf("Rehearse: %v", err)
	}
	return id, call
}

// TestEditedApproveRehashesAndComments: editing an intent's message text before approving
// recomputes call_sha256, executes the edited call once, and posts a system comment on the
// issue noting "đã sửa trước khi gửi" (durable spec 5.3, 5.6 item 2, plan B4).
func TestEditedApproveRehashesAndComments(t *testing.T) {
	fx := newIntentServiceFixture(t)
	ctx := context.Background()

	id, call := fx.rehearseEmail(t, "Original draft body.")
	before, err := fx.intents.Get(ctx, fx.ws, id)
	if err != nil {
		t.Fatalf("Get before approve: %v", err)
	}
	if before.CallSha256 != CallSHA256(call) {
		t.Fatalf("initial sha = %q, want %q", before.CallSha256, CallSHA256(call))
	}

	edited := connectedapps.CanonicalCall{
		Tool:      call.Tool,
		AccountID: call.AccountID,
		Args:      map[string]any{"to": "customer@example.com", "subject": "Quote", "body": "Edited final body."},
	}
	after, err := fx.intents.Approve(ctx, fx.ws, fx.user, id, &edited, "adjusted wording")
	if err != nil {
		t.Fatalf("Approve edited: %v", err)
	}
	if after.State != "executed" {
		t.Fatalf("state = %q, want executed", after.State)
	}
	wantSHA := CallSHA256(edited)
	if after.CallSha256 == before.CallSha256 || after.CallSha256 != wantSHA {
		t.Fatalf("call_sha256 after edit = %q, want %q (different from %q)", after.CallSha256, wantSHA, before.CallSha256)
	}
	if len(fx.exec.calls) != 1 || fx.exec.calls[0].Args["body"] != "Edited final body." {
		t.Fatalf("executed calls = %+v, want 1 call with edited body", fx.exec.calls)
	}

	var content string
	if err := fx.Pool.QueryRow(ctx,
		`SELECT content FROM comment WHERE issue_id = $1 AND author_type = 'system' ORDER BY created_at DESC LIMIT 1`,
		fx.issue).Scan(&content); err != nil {
		t.Fatalf("read system comment: %v", err)
	}
	if !strings.Contains(content, "đã sửa trước khi gửi") {
		t.Fatalf("system comment = %q, want it to note %q", content, "đã sửa trước khi gửi")
	}
}

// TestExpireWakesAgentOnce: after 72 hours a pending intent moves to expired, its effect
// event becomes blocked, and the agent is woken exactly once even if Expire runs again
// (durable spec 5.3, 5.6 item 4, plan B4).
func TestExpireWakesAgentOnce(t *testing.T) {
	fx := newIntentServiceFixture(t)
	ctx := context.Background()

	id, _ := fx.rehearseEmail(t, "Waiting for approval.")
	intentIDStr := util.UUIDToString(id)

	// Before 72 hours, Expire leaves this intent pending and wakes nobody.
	fx.now = fx.now.Add(71 * time.Hour)
	if _, err := fx.intents.Expire(ctx); err != nil {
		t.Fatalf("Expire before 72h: %v", err)
	}
	it, err := fx.intents.Get(ctx, fx.ws, id)
	if err != nil {
		t.Fatalf("Get before 72h: %v", err)
	}
	if it.State != "pending" {
		t.Fatalf("state before 72h = %q, want pending", it.State)
	}
	wakeCount := func() int {
		t.Helper()
		return fx.Count(t,
			`SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND trigger_evidence_kind = 'effect_intent' AND trigger_evidence_ref_id = $3`,
			fx.issue, fx.agent, intentIDStr)
	}
	if n := wakeCount(); n != 0 {
		t.Fatalf("wakeup tasks before 72h = %d, want 0", n)
	}

	// Past 72 hours, Expire moves it to expired, blocks its event, and wakes the agent once.
	fx.now = fx.now.Add(2 * time.Hour)
	if _, err := fx.intents.Expire(ctx); err != nil {
		t.Fatalf("Expire after 72h: %v", err)
	}
	it, err = fx.intents.Get(ctx, fx.ws, id)
	if err != nil {
		t.Fatalf("Get after 72h: %v", err)
	}
	if it.State != "expired" {
		t.Fatalf("state after 72h = %q, want expired", it.State)
	}
	if n := fx.Count(t, `SELECT count(*) FROM effect_event WHERE intent_id = $1 AND status = 'blocked'`, intentIDStr); n != 1 {
		t.Fatalf("blocked effect events for expired intent = %d, want 1", n)
	}
	if n := wakeCount(); n != 1 {
		t.Fatalf("wakeup tasks after first Expire = %d, want 1", n)
	}

	// A second Expire run must not wake the agent again.
	fx.now = fx.now.Add(time.Hour)
	if _, err := fx.intents.Expire(ctx); err != nil {
		t.Fatalf("second Expire: %v", err)
	}
	if n := wakeCount(); n != 1 {
		t.Fatalf("wakeup tasks after second Expire = %d, want 1", n)
	}
	if len(fx.exec.calls) != 0 {
		t.Fatalf("expired intent executed %d calls, want 0", len(fx.exec.calls))
	}
}
