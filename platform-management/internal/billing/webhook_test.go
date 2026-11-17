package billing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

const webhookTestPath = "/api/payments/webhook/sepay"

// fakeNoticeStore records what the listener writes. Insert keeps one row per
// (provider, event id) like the real table; a repeat returns the row with
// inserted=false.
type fakeNoticeStore struct {
	mu          sync.Mutex
	inserts     []paymentnotice.NewNotice
	transitions []paymentnotice.Change
	rows        map[string]paymentnotice.Notice
	insertErr   error
	events      []string // "insert" and "reconcile", in call order
}

func newFakeNoticeStore() *fakeNoticeStore {
	return &fakeNoticeStore{rows: map[string]paymentnotice.Notice{}}
}

func (f *fakeNoticeStore) Insert(_ context.Context, n paymentnotice.NewNotice) (paymentnotice.Notice, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserts = append(f.inserts, n)
	f.events = append(f.events, "insert")
	if f.insertErr != nil {
		return paymentnotice.Notice{}, false, f.insertErr
	}
	key := n.Provider + "/" + n.ProviderEventID
	if row, ok := f.rows[key]; ok {
		return row, false, nil
	}
	row := paymentnotice.Notice{
		ID: "notice-" + n.ProviderEventID, Provider: n.Provider, ProviderEventID: n.ProviderEventID,
		State: paymentnotice.StateReceived, TransferCode: n.TransferCode, Amount: n.Amount,
		Currency: n.Currency, BodySHA256: n.BodySHA256,
	}
	f.rows[key] = row
	return row, true, nil
}

func (f *fakeNoticeStore) Transition(_ context.Context, c paymentnotice.Change) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transitions = append(f.transitions, c)
	return nil
}

func (f *fakeNoticeStore) insertCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inserts)
}

// fakeReconciler records Reconcile calls in the shared event log.
type fakeReconciler struct {
	store *fakeNoticeStore
	ids   []string
	err   error
}

func (f *fakeReconciler) Reconcile(_ context.Context, id string) (paymentnotice.Notice, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	f.ids = append(f.ids, id)
	f.store.events = append(f.store.events, "reconcile")
	return paymentnotice.Notice{ID: id}, f.err
}

func newWebhookTestHandler(t *testing.T, store *fakeNoticeStore, maxBytes int64, logs *bytes.Buffer) http.Handler {
	t.Helper()
	h, _ := newWebhookTestHandlerRec(t, store, maxBytes, logs)
	return h
}

func newWebhookTestHandlerRec(t *testing.T, store *fakeNoticeStore, maxBytes int64, logs *bytes.Buffer) (http.Handler, *fakeReconciler) {
	t.Helper()
	rec := &fakeReconciler{store: store}
	reg, err := NewRegistry("vnd=sepay", testBank(), func(string) string { return testSePaySecret })
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(logs, nil))
	return NewWebhookHandler(WebhookDeps{Registry: reg, Notices: store, Reconciler: rec, MaxBytes: maxBytes, Log: log}), rec
}

func postWebhook(h http.Handler, path string, body []byte, hdr http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestListenerServesOnlyWebhookPath: the listener answers POST on the webhook
// route and nothing else. Every other path, method, or unconfigured provider is
// a 404, including the wrong method on the right path (not the mux's 405).
func TestListenerServesOnlyWebhookPath(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")

	// Control: the signed POST on the real route succeeds, so the 404s below
	// are not a handler that always refuses.
	if rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body)); rec.Code != http.StatusOK {
		t.Fatalf("control POST on webhook route = %d, want 200", rec.Code)
	}

	cases := []struct {
		name, method, path string
	}{
		{"GET on webhook route", http.MethodGet, webhookTestPath},
		{"PUT on webhook route", http.MethodPut, webhookTestPath},
		{"root", http.MethodPost, "/"},
		{"platform api", http.MethodPost, "/api/platform/me"},
		{"internal route", http.MethodPost, "/api/v1/internal/entitlement-policies/x"},
		{"unconfigured provider", http.MethodPost, "/api/payments/webhook/payos"},
		{"empty provider", http.MethodPost, "/api/payments/webhook/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, bytes.NewReader(body))
			for k, vs := range sepayHeaders(testSePaySecret, body) {
				req.Header[k] = vs
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d, want 404", c.method, c.path, rec.Code)
			}
		})
	}
	if n := store.insertCount(); n != 1 {
		t.Fatalf("404 requests reached the store: inserts = %d, want only the control's 1", n)
	}
}

// TestBadSignature401NoWriteNoBodyLog: a bad or missing signature is a 401 with
// the contract code, writes nothing, and the log never carries the body.
func TestBadSignature401NoWriteNoBodyLog(t *testing.T) {
	body := readSePayFixture(t, "incoming.json")
	wrong := sepayHeaders("not-the-secret", body)
	missing := http.Header{}

	for name, hdr := range map[string]http.Header{"wrong secret": wrong, "missing headers": missing} {
		t.Run(name, func(t *testing.T) {
			store := newFakeNoticeStore()
			var logs bytes.Buffer
			h := newWebhookTestHandler(t, store, 0, &logs)
			rec := postWebhook(h, webhookTestPath, body, hdr)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), `"payment_signature_invalid"`) {
				t.Fatalf("body = %s, want code payment_signature_invalid", rec.Body.String())
			}
			if n := store.insertCount(); n != 0 {
				t.Fatalf("inserts = %d, want 0", n)
			}
			if len(store.transitions) != 0 {
				t.Fatalf("transitions = %d, want 0", len(store.transitions))
			}
			if strings.Contains(logs.String(), "MULABCDEFGH") || strings.Contains(logs.String(), "92704") {
				t.Fatalf("log carries the body: %s", logs.String())
			}
		})
	}
}

// TestUnsignedBodyIsNotParsed: a body that would not parse is still a 401 when
// the signature is wrong, so the signature check comes before any parsing.
func TestUnsignedBodyIsNotParsed(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	garbage := []byte("{not json")
	rec := postWebhook(h, webhookTestPath, garbage, sepayHeaders("not-the-secret", garbage))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (signature before parse)", rec.Code)
	}
}

// TestBodyOverLimitRejected: a body over the limit is refused before it is
// parsed or stored, even when it is correctly signed.
func TestBodyOverLimitRejected(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 16, &logs)
	body := readSePayFixture(t, "incoming.json")
	rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if n := store.insertCount(); n != 0 {
		t.Fatalf("inserts = %d, want 0", n)
	}
}

// TestValidNoticeStoredWithEventID: a signed incoming transfer is stored once,
// with the provider event id, amount, currency and transfer code the adapter read.
func TestValidNoticeStoredWithEventID(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")
	rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"success":true}` {
		t.Fatalf("body = %s, want {\"success\":true}", got)
	}
	if len(store.inserts) != 1 {
		t.Fatalf("inserts = %d, want 1", len(store.inserts))
	}
	n := store.inserts[0]
	if n.Provider != ProviderSePay || n.ProviderEventID != "92704" || n.Amount != 491700 ||
		n.Currency != "VND" || n.TransferCode != "MULABCDEFGH" {
		t.Fatalf("stored notice = %+v", n)
	}
	sum := sha256.Sum256(body)
	if n.BodySHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("body hash = %s, want SHA-256 of the raw body", n.BodySHA256)
	}
}

// TestDuplicateEventAnsweredWithoutSecondRecord: a redelivery of the same event
// is answered 200 with the same body and adds no row and no transition. Nothing
// here applies a payment, so a duplicate cannot apply one twice.
func TestDuplicateEventAnsweredWithoutSecondRecord(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")
	for i := 0; i < 2; i++ {
		rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"success":true}` {
			t.Fatalf("delivery %d = %d %s, want 200 success", i+1, rec.Code, rec.Body.String())
		}
	}
	if len(store.inserts) != 2 || len(store.rows) != 1 {
		t.Fatalf("inserts = %d rows = %d, want 2 attempts and 1 row", len(store.inserts), len(store.rows))
	}
	if len(store.transitions) != 0 {
		t.Fatalf("transitions = %d, want 0 on a duplicate", len(store.transitions))
	}
}

// TestUnparsableSignedBodyStoredForReview: a correctly signed body that the
// adapter cannot read is recorded for staff, keyed by the body's hash, and the
// provider gets 200 so it stops retrying.
func TestUnparsableSignedBodyStoredForReview(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	body := []byte(`{"transferType":"in","transferAmount":100}`)
	rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(store.inserts) != 1 {
		t.Fatalf("inserts = %d, want 1", len(store.inserts))
	}
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])
	if store.inserts[0].ProviderEventID != want {
		t.Fatalf("event id = %s, want the body hash %s", store.inserts[0].ProviderEventID, want)
	}
	if len(store.transitions) != 1 || store.transitions[0].To != paymentnotice.StateNeedsReview ||
		store.transitions[0].ReviewReason != "unparsable" {
		t.Fatalf("transitions = %+v, want one move to needs_review with reason unparsable", store.transitions)
	}
}

// TestOutgoingTransferAckedNotStored: an outgoing transfer is not a payment; it
// is acknowledged and not recorded.
func TestOutgoingTransferAckedNotStored(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	body := readSePayFixture(t, "outgoing.json")
	rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if n := store.insertCount(); n != 0 {
		t.Fatalf("inserts = %d, want 0", n)
	}
}

// TestStoreErrorReturns503: a database failure is a 503, so the provider retries
// the same event later.
func TestStoreErrorReturns503(t *testing.T) {
	store := newFakeNoticeStore()
	store.insertErr = errors.New("database down")
	var logs bytes.Buffer
	h := newWebhookTestHandler(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")
	rec := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func wantNoReconcile(t *testing.T, rec *fakeReconciler) {
	t.Helper()
	if len(rec.ids) != 0 {
		t.Fatalf("reconcile calls = %v, want none", rec.ids)
	}
}

// TestAcceptedNoticeIsReconciledAfterTheStoreWrite: a new notice is reconciled
// once, by its stored ID, after the insert.
func TestAcceptedNoticeIsReconciledAfterTheStoreWrite(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h, rec := newWebhookTestHandlerRec(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")
	resp := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
	if len(rec.ids) != 1 || rec.ids[0] != "notice-92704" {
		t.Fatalf("reconcile calls = %v, want one for notice-92704", rec.ids)
	}
	if got := strings.Join(store.events, ","); got != "insert,reconcile" {
		t.Fatalf("event order = %s, want insert,reconcile", got)
	}
}

// TestDuplicateNoticeIsReconciledOnceMore: a redelivery is reconciled too, so a
// notice left in received by a crashed run is picked up.
func TestDuplicateNoticeIsReconciledOnceMore(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h, rec := newWebhookTestHandlerRec(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")
	postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	rec.ids = nil
	resp := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
	if len(rec.ids) != 1 || rec.ids[0] != "notice-92704" {
		t.Fatalf("reconcile calls on duplicate = %v, want one for notice-92704", rec.ids)
	}
}

// TestReviewPathDoesNotReconcile: a notice routed to needs_review is not reconciled.
func TestReviewPathDoesNotReconcile(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h, rec := newWebhookTestHandlerRec(t, store, 0, &logs)
	body := []byte(`{"id":1,"content":"MULABCDEFGH MULJKMNPQRS","transferType":"in","transferAmount":100,"transactionDate":"2026-11-17 09:30:15"}`)
	resp := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
	if len(store.transitions) != 1 || store.transitions[0].ReviewReason != "ambiguous_code" {
		t.Fatalf("transitions = %+v, want one ambiguous_code review", store.transitions)
	}
	wantNoReconcile(t, rec)
}

// TestStoreErrorSkipsReconcileAndAnswers503: nothing is stored, so nothing is reconciled.
func TestStoreErrorSkipsReconcileAndAnswers503(t *testing.T) {
	store := newFakeNoticeStore()
	store.insertErr = errors.New("database down")
	var logs bytes.Buffer
	h, rec := newWebhookTestHandlerRec(t, store, 0, &logs)
	body := readSePayFixture(t, "incoming.json")
	resp := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if resp.Code != http.StatusServiceUnavailable || !strings.Contains(resp.Body.String(), `"payment_store_unavailable"`) {
		t.Fatalf("response = %d %s, want 503 payment_store_unavailable", resp.Code, resp.Body.String())
	}
	wantNoReconcile(t, rec)
}

// TestReconcileErrorStillAnswers200: the row is stored, so a reconcile failure
// does not change the answer, and the log carries no body or signature.
func TestReconcileErrorStillAnswers200(t *testing.T) {
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	h, rec := newWebhookTestHandlerRec(t, store, 0, &logs)
	rec.err = errors.New("reconcile store down")
	body := readSePayFixture(t, "incoming.json")
	hdr := sepayHeaders(testSePaySecret, body)
	resp := postWebhook(h, webhookTestPath, body, hdr)
	if resp.Code != http.StatusOK || strings.TrimSpace(resp.Body.String()) != `{"success":true}` {
		t.Fatalf("response = %d %s, want 200 success", resp.Code, resp.Body.String())
	}
	if len(rec.ids) != 1 {
		t.Fatalf("reconcile calls = %v, want 1", rec.ids)
	}
	out := logs.String()
	if !strings.Contains(out, "reconcile store down") || !strings.Contains(out, "notice-92704") {
		t.Fatalf("log lacks the reconcile error or notice id: %s", out)
	}
	for _, secret := range []string{"MULABCDEFGH", "491700", hdr.Get("X-SePay-Signature"), testSePaySecret} {
		if strings.Contains(out, secret) {
			t.Fatalf("log carries %q: %s", secret, out)
		}
	}
}

// TestWebhookEndToEndOnRealDB: a signed notice for a pending order is stored,
// applied, and audited once. Needs the test database; skipped otherwise.
func TestWebhookEndToEndOnRealDB(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	var eventID int64
	if err := f.pool.QueryRow(context.Background(), `SELECT (random()*1e12)::bigint + 1e12`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.pool.Exec(context.Background(), `DELETE FROM payment_notice WHERE provider = $1 AND provider_event_id = $2`,
			ProviderSePay, fmt.Sprint(eventID))
	})
	body := []byte(fmt.Sprintf(`{"id":%d,"content":%q,"transactionDate":"2026-11-17 09:30:15","transferType":"in","transferAmount":%d}`,
		eventID, o.TransferCode, o.Total()))

	reg, err := NewRegistry("vnd=sepay", testBank(), func(string) string { return testSePaySecret })
	if err != nil {
		t.Fatal(err)
	}
	h := NewWebhookHandler(WebhookDeps{Registry: reg, Notices: f.notices, Reconciler: f.rec, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
	resp := postWebhook(h, webhookTestPath, body, sepayHeaders(testSePaySecret, body))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", resp.Code, resp.Body.String())
	}

	var id string
	var state paymentnotice.State
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id::text, state FROM payment_notice WHERE provider = $1 AND provider_event_id = $2`,
		ProviderSePay, fmt.Sprint(eventID)).Scan(&id, &state); err != nil {
		t.Fatal(err)
	}
	if state != paymentnotice.StateApplied {
		t.Fatalf("notice state = %s, want applied", state)
	}
	if got := f.orderStatus(o.ID); got != StatusPaid {
		t.Fatalf("order status = %s, want paid", got)
	}
	if n := f.auditCount("billing.order_paid", o.ID); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", n)
	}
}
