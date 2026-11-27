package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	sdk "github.com/multica-ai/multica/server/pkg/composio"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// intentFixture is the connected apps fixture with the intent service wired to the
// gateway and to the handler. The gmail personal account is connected, and the agent
// has gmail at write level.
func intentFixture(t *testing.T) (*appsFixture, *service.IntentService) {
	t.Helper()
	intents := &service.IntentService{Queries: testHandler.Queries, TxStarter: testPool, Flags: effectFlagsOn()}
	f := newAppsFixture(t, withEffectGate(true), func(cfg *connectedapps.Config) { cfg.Intents = intents })
	intents.Apps = f.h.ConnectedApps
	f.h.durable.Intents = intents
	f.executionAccount(t, "personal")
	if w := f.putAgentApps(t, testUserID, []map[string]string{{"toolkit_slug": "gmail", "account_scope": "personal", "level": "write"}}); w.Code != 200 {
		t.Fatalf("attach app: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_intent WHERE task_id = $1`, f.taskID)
		testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id = $1`, f.taskID)
	})
	return f, intents
}

// intentRequest is a decision request as the authenticated group passes it on.
func intentRequest(t *testing.T, path, userID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	req.Header.Set("X-User-ID", userID)
	return req
}

// rehearsedSend has the agent send an email. At L1 the policy rehearses it, so the
// gateway answers with an intent id and nothing is sent.
func rehearsedSend(t *testing.T, f *appsFixture, body string) string {
	t.Helper()
	text, isErr := sendRehearsal(t, f, body)
	if !isErr {
		t.Fatalf("send was not held: %s", text)
	}
	var out struct {
		Status   string `json:"status"`
		IntentID string `json:"intent_id"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil || out.Status != "awaiting_approval" || out.IntentID == "" {
		t.Fatalf("rehearsal answer %q: %v", text, err)
	}
	return out.IntentID
}

func sendRehearsal(t *testing.T, f *appsFixture, body string) (string, bool) {
	t.Helper()
	args := map[string]any{
		"user_id": "me", "to": "customer@example.com", "subject": "Quote", "body": body,
		"_rationale": "the customer asked for the quote",
	}
	_, text, isErr := f.gateway(t, f.token(t), "apps_run_tool", map[string]any{"tool": "GMAIL_SEND_EMAIL", "arguments": args})
	return text, isErr
}

func intentRow(t *testing.T, id string) (state string, sha string) {
	t.Helper()
	testPool.QueryRow(context.Background(), `SELECT state, call_sha256 FROM effect_intent WHERE id = $1`, id).Scan(&state, &sha)
	return state, sha
}

func parseIntentID(t *testing.T, id string) pgtype.UUID {
	t.Helper()
	u, err := util.ParseUUID(id)
	if err != nil {
		t.Fatalf("intent id %q: %v", id, err)
	}
	return u
}

// TestRehearseDoesNotCallGateway: a send the policy rehearses is not sent. The intent
// holds its preview, which names the recipient, and the rationale stays out of it.
func TestRehearseDoesNotCallGateway(t *testing.T) {
	f, _ := intentFixture(t)
	id := rehearsedSend(t, f, "Here is the quote.")

	if n := len(f.sdk.executions()); n != 0 {
		t.Fatalf("provider calls after a rehearsed send = %d, want 0", n)
	}
	state, _ := intentRow(t, id)
	if state != "pending" {
		t.Fatalf("intent state %q, want pending", state)
	}
	var preview struct {
		Kind       string   `json:"kind"`
		Recipients []string `json:"recipients"`
	}
	var raw string
	testPool.QueryRow(context.Background(), `SELECT preview::text FROM effect_intent WHERE id = $1`, id).Scan(&raw)
	if err := json.Unmarshal([]byte(raw), &preview); err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Kind != "email" || len(preview.Recipients) != 1 || preview.Recipients[0] != "customer@example.com" {
		t.Fatalf("preview = %s", raw)
	}
	if strings.Contains(raw, "the customer asked") {
		t.Fatalf("preview carries the agent's rationale: %s", raw)
	}
}

// TestApproveExecutesExactCall: approving runs the rehearsed call once, with the same
// arguments, and moves the intent and its event to executed.
func TestApproveExecutesExactCall(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Here is the quote.")

	if _, err := svc.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id), nil, "looks right"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	execs := f.sdk.executions()
	if len(execs) != 1 {
		t.Fatalf("provider calls after approval = %d, want 1", len(execs))
	}
	if execs[0].Arguments["body"] != "Here is the quote." || execs[0].Arguments["to"] != "customer@example.com" {
		t.Fatalf("executed arguments %+v differ from the rehearsed call", execs[0].Arguments)
	}
	if _, has := execs[0].Arguments["_rationale"]; has {
		t.Fatal("the rationale was sent to the app")
	}
	if state, _ := intentRow(t, id); state != "executed" {
		t.Fatalf("intent state %q, want executed", state)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE intent_id=$1 AND status='executed'`, id); n != 1 {
		t.Fatalf("event for the intent: %d executed, want 1", n)
	}
}

// TestApproveIntentConcurrentlyExecutesOnce: two approvals at once run the call once.
// The loser answers not pending.
func TestApproveIntentConcurrentlyExecutesOnce(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Race me.")
	ws, user, intent := parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Approve(context.Background(), ws, user, intent, nil, "")
		}(i)
	}
	wg.Wait()

	won, lost := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, service.ErrIntentNotPending):
			lost++
		default:
			t.Fatalf("unexpected approval error: %v", err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatalf("approvals won=%d lost=%d, want 1 and 1", won, lost)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Fatalf("provider calls = %d, want 1", n)
	}
}

// TestEditedApproveRehashesAndExecutesEdit: an edit to the message text changes the
// call that runs and its hash. Only the text may change.
func TestEditedApproveRehashesAndExecutesEdit(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Draft text.")
	_, originalSHA := intentRow(t, id)

	var raw string
	testPool.QueryRow(context.Background(), `SELECT call::text FROM effect_intent WHERE id = $1`, id).Scan(&raw)
	var call connectedapps.CanonicalCall
	if err := json.Unmarshal([]byte(raw), &call); err != nil {
		t.Fatalf("decode call: %v", err)
	}
	call.Args["body"] = "Edited text."
	if _, err := svc.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id), &call, "edited"); err != nil {
		t.Fatalf("Approve edited: %v", err)
	}
	execs := f.sdk.executions()
	if len(execs) != 1 || execs[0].Arguments["body"] != "Edited text." {
		t.Fatalf("executed %+v, want the edited body", execs)
	}
	_, newSHA := intentRow(t, id)
	if newSHA == originalSHA || newSHA != service.CallSHA256(call) {
		t.Fatalf("hash after edit = %s, want the edited call's hash (original %s)", newSHA, originalSHA)
	}
}

// TestEditRefusesOtherArguments: an edit that changes the recipient is refused.
func TestEditRefusesOtherArguments(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Keep me.")
	var raw string
	testPool.QueryRow(context.Background(), `SELECT call::text FROM effect_intent WHERE id = $1`, id).Scan(&raw)
	var call connectedapps.CanonicalCall
	json.Unmarshal([]byte(raw), &call)
	call.Args["to"] = "someone-else@example.com"
	_, err := svc.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id), &call, "")
	if !errors.Is(err, service.ErrIntentEditNotAllowed) {
		t.Fatalf("edit of the recipient: %v, want ErrIntentEditNotAllowed", err)
	}
	if n := len(f.sdk.executions()); n != 0 {
		t.Fatalf("refused edit still ran %d calls", n)
	}
}

// TestBatchApproveFive: five rehearsed sends approved in one request run five calls
// and settle five events.
func TestBatchApproveFive(t *testing.T) {
	f, _ := intentFixture(t)
	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		ids = append(ids, rehearsedSend(t, f, "Batch "+strings.Repeat("x", i+1)))
	}
	body, _ := json.Marshal(map[string]any{"ids": ids})
	req := intentRequest(t, "/api/intents/approve-batch", testUserID, string(body))
	w := httptest.NewRecorder()
	f.h.ApproveIntentBatch(w, req)
	if w.Code != 200 {
		t.Fatalf("batch: %d %s", w.Code, w.Body.String())
	}
	if n := len(f.sdk.executions()); n != 5 {
		t.Fatalf("provider calls = %d, want 5", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND source='connected_app' AND status='executed' AND intent_id IS NOT NULL`, f.taskID); n != 5 {
		t.Fatalf("executed events = %d, want 5", n)
	}
}

// TestDecidedIntentAnswers409: a second approval of an executed intent is refused and
// runs nothing.
func TestDecidedIntentAnswers409(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Once only.")
	if _, err := svc.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id), nil, ""); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	req := intentRequest(t, "/api/intents/"+id+"/approve", testUserID, "")
	req = testutil.WithURLParams(req, "id", id)
	w := httptest.NewRecorder()
	f.h.ApproveIntent(w, req)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "intent_not_pending") {
		t.Fatalf("second approve: %d %s, want 409 intent_not_pending", w.Code, w.Body.String())
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Fatalf("provider calls = %d, want 1", n)
	}
}

// TestRejectNeedsNoteAndHoldsTheCall: a rejection needs a note, and then nothing runs.
func TestRejectNeedsNoteAndHoldsTheCall(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "No thanks.")
	ws, user, intent := parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id)
	if _, err := svc.Reject(context.Background(), ws, user, intent, "  "); !errors.Is(err, service.ErrIntentNoteRequired) {
		t.Fatalf("reject without note: %v", err)
	}
	if _, err := svc.Reject(context.Background(), ws, user, intent, "Not to this customer."); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if state, _ := intentRow(t, id); state != "rejected" {
		t.Fatalf("state %q, want rejected", state)
	}
	if n := len(f.sdk.executions()); n != 0 {
		t.Fatalf("rejected intent ran %d calls", n)
	}
}

// TestExpireHoldsNothingPastItsTime: an intent past its expiry is expired, its event
// is blocked, and it is not run.
func TestExpireHoldsNothingPastItsTime(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Too late.")
	if _, err := testPool.Exec(context.Background(), `UPDATE effect_intent SET expires_at = now() - interval '1 hour' WHERE id = $1`, id); err != nil {
		t.Fatalf("age intent: %v", err)
	}
	if _, err := svc.Expire(context.Background()); err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if state, _ := intentRow(t, id); state != "expired" {
		t.Fatalf("state %q, want expired", state)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE intent_id=$1 AND status='blocked'`, id); n != 1 {
		t.Fatalf("event after expiry: %d blocked, want 1", n)
	}
	if _, err := svc.Expire(context.Background()); err != nil {
		t.Fatalf("second Expire: %v", err)
	}
	if state, _ := intentRow(t, id); state != "expired" {
		t.Fatalf("second expiry changed state to %q", state)
	}
}

// TestIntentFlagOffStopsWrites: while durable_effects is off, approval runs nothing.
func TestIntentFlagOffStopsWrites(t *testing.T) {
	f, svc := intentFixture(t)
	id := rehearsedSend(t, f, "Flag off.")
	off := featureflag.NewStaticProvider()
	off.Set(featureflags.DurableEffects, featureflag.Rule{Default: false})
	svc.Flags = featureflag.NewService(off)
	if _, err := svc.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseIntentID(t, id), nil, ""); !errors.Is(err, service.ErrEffectsOff) {
		t.Fatalf("approve with flag off: %v, want ErrEffectsOff", err)
	}
	if n := len(f.sdk.executions()); n != 0 {
		t.Fatalf("flag off ran %d calls", n)
	}
	if state, _ := intentRow(t, id); state != "pending" {
		t.Fatalf("state %q, want pending", state)
	}
}

func TestExecuteIntentChecksAccountBeforeProvider(t *testing.T) {
	f := newAppsFixture(t)
	_, _, _, account := f.executionAccount(t, "personal")
	call := connectedapps.CanonicalCall{Tool: "GMAIL_SEND_EMAIL", AccountID: account + "-old", Args: map[string]any{"to": "customer@example.com"}}
	_, err := f.h.ConnectedApps.ExecuteIntent(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), "gmail", call)
	if !errors.Is(err, connectedapps.ErrIntentAccountChanged) {
		t.Errorf("changed account error = %v, want ErrIntentAccountChanged", err)
	}
	if n := len(f.sdk.executions()); n != 0 {
		t.Errorf("changed account provider calls = %d, want 0", n)
	}
	call.AccountID = account
	if _, err := f.h.ConnectedApps.ExecuteIntent(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), "gmail", call); err != nil {
		t.Errorf("matching account: %v", err)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Errorf("matching account provider calls = %d, want 1 total", n)
	}
}

// TestShadowGatewayWriteBlocked: a shadow task calling a write or send tool through the
// connected apps gateway receives block even when the agent holds write permission on the
// toolkit, and the provider is never called; read tools still run (durable spec 4.8 item 3,
// 9.4 D18, 9.7 item 3, plan D3).
func TestShadowGatewayWriteBlocked(t *testing.T) {
	old := appsFakeGmailTools
	appsFakeGmailTools = append(append([]sdk.Tool{}, old...),
		sdk.Tool{Slug: "GMAIL_CREATE_DRAFT", Name: "Create draft", Toolkit: sdk.Toolkit{Slug: "gmail"}})
	t.Cleanup(func() { appsFakeGmailTools = old })

	f, _ := intentFixture(t)
	if _, err := testPool.Exec(context.Background(),
		`UPDATE agent_task_queue SET context = '{"durable_kind":"shadow"}'::jsonb WHERE id = $1`, f.taskID); err != nil {
		t.Fatalf("mark task shadow: %v", err)
	}
	token := f.token(t)

	for _, tool := range []string{"GMAIL_CREATE_DRAFT", "GMAIL_SEND_EMAIL", "GMAIL_DELETE_MESSAGE"} {
		_, text, isErr := f.gateway(t, token, "apps_run_tool", map[string]any{
			"tool":      tool,
			"arguments": map[string]any{"user_id": "me", "to": "customer@example.com", "subject": "Test", "body": "Hi"},
		})
		if !isErr {
			t.Fatalf("shadow call to %s succeeded (%s), want block", tool, text)
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE task_id=$1 AND target=$2 AND decision='block' AND status='blocked'`,
			f.taskID, "gmail/"+tool); n != 1 {
			t.Fatalf("shadow call to %s logged %d blocked events, want 1", tool, n)
		}
	}
	if n := len(f.sdk.executions()); n != 0 {
		t.Fatalf("provider calls after blocked shadow writes = %d, want 0", n)
	}

	// Read calls remain allowed for a shadow task.
	if _, text, isErr := f.gateway(t, token, "apps_run_tool", map[string]any{
		"tool":      "GMAIL_FETCH_EMAILS",
		"arguments": map[string]any{"user_id": "me"},
	}); isErr {
		t.Fatalf("shadow read failed: %s", text)
	}
	if n := len(f.sdk.executions()); n != 1 {
		t.Fatalf("provider calls after shadow read = %d, want 1", n)
	}
}
