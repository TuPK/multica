package console

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

const (
	noticeA = "99999999-0000-0000-0000-00000000000a"
	noticeB = "99999999-0000-0000-0000-00000000000b"
)

// fakeNoticeReader adds the payment notice and e-invoice reads to fakeReader.
type fakeNoticeReader struct {
	*fakeReader
	notices    []paymentnotice.Notice
	candidates []NoticeCandidate
	invoices   []EinvoiceRow

	lastState      paymentnotice.State
	lastLimit      int
	lastCandNotice paymentnotice.Notice
	lastCandLimit  int
	lastInvStates  []invoice.State
	lastInvLimit   int
}

func (r *fakeNoticeReader) ListPaymentNotices(_ context.Context, state paymentnotice.State, limit int) ([]paymentnotice.Notice, error) {
	r.lastState, r.lastLimit = state, limit
	var out []paymentnotice.Notice
	for _, n := range r.notices {
		if n.State == state {
			out = append(out, n)
		}
	}
	// The real store sorts; the fake sorts the same way so the test reads the order.
	oldestFirst := state == paymentnotice.StateNeedsReview
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].ReceivedAt.Before(out[i].ReceivedAt) == oldestFirst {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, r.err
}

func (r *fakeNoticeReader) GetPaymentNotice(_ context.Context, id string) (paymentnotice.Notice, bool, error) {
	for _, n := range r.notices {
		if n.ID == id {
			return n, true, r.err
		}
	}
	return paymentnotice.Notice{}, false, r.err
}

func (r *fakeNoticeReader) NoticeCandidates(_ context.Context, n paymentnotice.Notice, limit int) ([]NoticeCandidate, error) {
	r.lastCandNotice, r.lastCandLimit = n, limit
	return r.candidates, r.err
}

func (r *fakeNoticeReader) ListEinvoices(_ context.Context, states []invoice.State, limit int) ([]EinvoiceRow, error) {
	r.lastInvStates, r.lastInvLimit = states, limit
	return r.invoices, r.err
}

// fakeReconciler records the decisions and answers with canned results.
type fakeReconciler struct {
	notice paymentnotice.Notice
	err    error

	resolves []resolveCall
	dismiss  []dismissCall
}

type resolveCall struct {
	actor                  billing.Actor
	noticeID, orderID, why string
}

type dismissCall struct {
	actor         billing.Actor
	noticeID, why string
}

func (f *fakeReconciler) Resolve(_ context.Context, a billing.Actor, noticeID, orderID, reason string) (paymentnotice.Notice, error) {
	f.resolves = append(f.resolves, resolveCall{a, noticeID, orderID, reason})
	return f.notice, f.err
}

func (f *fakeReconciler) Dismiss(_ context.Context, a billing.Actor, noticeID, reason string) (paymentnotice.Notice, error) {
	f.dismiss = append(f.dismiss, dismissCall{a, noticeID, reason})
	return f.notice, f.err
}

// withNotices swaps the environment's reader and reconciler for the fakes.
func withNotices(e *env) (*fakeNoticeReader, *fakeReconciler) {
	nr := &fakeNoticeReader{fakeReader: e.reader}
	rc := &fakeReconciler{}
	h := e.h.(*Handler)
	h.Reader, h.Reconciler = nr, rc
	return nr, rc
}

func reviewNotice(id string, received time.Time) paymentnotice.Notice {
	return paymentnotice.Notice{
		ID: id, Provider: "sepay", ProviderEventID: "ev-" + id[len(id)-1:], State: paymentnotice.StateNeedsReview,
		ReviewReason: "amount_mismatch", TransferCode: "MULABCD1234", Amount: 660000, Currency: "VND",
		BodySHA256: "deadbeef", ReceivedAt: received,
	}
}

func TestNoticeListDefaultsToNeedsReviewOldestFirst(t *testing.T) {
	e := newEnv(t)
	nr, _ := withNotices(e)
	nr.notices = []paymentnotice.Notice{
		reviewNotice(noticeB, t0.Add(time.Hour)),
		reviewNotice(noticeA, t0),
		{ID: "99999999-0000-0000-0000-00000000000c", State: paymentnotice.StateApplied, ReceivedAt: t0},
	}
	rec := e.do(billID, "GET", "/api/platform/payment-notices", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if nr.lastState != paymentnotice.StateNeedsReview || nr.lastLimit != 50 {
		t.Errorf("asked for state %q limit %d, want needs_review and 50", nr.lastState, nr.lastLimit)
	}
	var body struct {
		Notices []map[string]any `json:"notices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notices) != 2 || body.Notices[0]["id"] != noticeA || body.Notices[1]["id"] != noticeB {
		t.Fatalf("notices = %s, want A then B", rec.Body)
	}
	first := body.Notices[0]
	for _, k := range []string{"id", "provider", "provider_event_id", "state", "transfer_code", "amount", "currency", "review_reason", "order_id", "received_at"} {
		if _, ok := first[k]; !ok {
			t.Errorf("item has no %q", k)
		}
	}
	if first["order_id"] != "" || first["amount"] != float64(660000) {
		t.Errorf("order_id %v amount %v", first["order_id"], first["amount"])
	}
	for _, banned := range []string{"body", "sha", "signature", "buyer"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), banned) {
			t.Errorf("response mentions %q: %s", banned, rec.Body)
		}
	}

	nr.lastLimit = 0
	if rec := e.do(adminID, "GET", "/api/platform/payment-notices?state=applied&limit=200", ""); rec.Code != 200 || nr.lastState != paymentnotice.StateApplied || nr.lastLimit != 200 {
		t.Errorf("state=applied limit=200: %d %s, state %q limit %d", rec.Code, rec.Body, nr.lastState, nr.lastLimit)
	}
}

func TestNoticeListRejectsUnknownState(t *testing.T) {
	e := newEnv(t)
	withNotices(e)
	for _, q := range []string{"state=bogus", "state=cancelled", "state=needs_review&state=applied", "limit=0", "limit=201", "limit=x"} {
		rec := e.do(billID, "GET", "/api/platform/payment-notices?"+q, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("?%s = %d %s, want 400", q, rec.Code, rec.Body)
		}
	}
}

func TestNoticeDetailShowsCandidatesByCodeAndAmount(t *testing.T) {
	e := newEnv(t)
	nr, _ := withNotices(e)
	nr.notices = []paymentnotice.Notice{reviewNotice(noticeA, t0)}
	nr.candidates = []NoticeCandidate{
		{OrderID: orderA, WorkspaceID: wsID, State: "pending", TransferCode: "MULABCD1234", Amount: 600000, VATAmount: 60000, Currency: "vnd", CreatedAt: t0, Match: "code"},
		{OrderID: orderB, WorkspaceID: wsID, State: "pending", TransferCode: "MULOTHER999", Amount: 600000, VATAmount: 60000, Currency: "vnd", CreatedAt: t0, Match: "amount"},
	}
	rec := e.do(billID, "GET", "/api/platform/payment-notices/"+noticeA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if nr.lastCandLimit != 20 || nr.lastCandNotice.TransferCode != "MULABCD1234" {
		t.Errorf("candidates asked with limit %d for %+v", nr.lastCandLimit, nr.lastCandNotice)
	}
	var body struct {
		ID         string `json:"id"`
		Amount     int64  `json:"amount"`
		Candidates []struct {
			OrderID     string `json:"order_id"`
			WorkspaceID string `json:"workspace_id"`
			State       string `json:"state"`
			Code        string `json:"transfer_code"`
			Amount      int64  `json:"amount"`
			VATAmount   int64  `json:"vat_amount"`
			Currency    string `json:"currency"`
			CreatedAt   string `json:"created_at"`
			Match       string `json:"match"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != noticeA || body.Amount != 660000 || len(body.Candidates) != 2 {
		t.Fatalf("body = %s", rec.Body)
	}
	c0, c1 := body.Candidates[0], body.Candidates[1]
	if c0.OrderID != orderA || c0.Match != "code" || c1.OrderID != orderB || c1.Match != "amount" || c0.VATAmount != 60000 || c0.WorkspaceID != wsID || c0.CreatedAt == "" {
		t.Errorf("candidates = %+v", body.Candidates)
	}

	// No candidates is an empty array, not null.
	nr.candidates = nil
	rec = e.do(billID, "GET", "/api/platform/payment-notices/"+noticeA, "")
	if !strings.Contains(rec.Body.String(), `"candidates":[]`) {
		t.Errorf("empty candidates = %s", rec.Body)
	}
}

func TestNoticeDetailUnknownIs404(t *testing.T) {
	e := newEnv(t)
	withNotices(e)
	wantError(t, e.do(billID, "GET", "/api/platform/payment-notices/"+noticeB, ""), http.StatusNotFound, "not_found")
	if rec := e.do(billID, "GET", "/api/platform/payment-notices/not-a-uuid", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad uuid = %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestResolveNeedsReason(t *testing.T) {
	e := newEnv(t)
	_, rc := withNotices(e)
	for _, body := range []string{`{"order_id":"` + orderA + `"}`, `{"order_id":"` + orderA + `","reason":"   "}`} {
		wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", body), http.StatusBadRequest, "reason_required")
	}
	// The reconciler's own refusal maps the same way.
	rc.err = billing.ErrReasonRequired
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+orderA+`","reason":"x"}`), http.StatusBadRequest, "reason_required")

	rc.err = nil
	rec := e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"nope","reason":"x"}`)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_order_id" {
		t.Errorf("bad order id = %d %s", rec.Code, rec.Body)
	}
	if len(rc.resolves) != 1 {
		t.Errorf("resolve calls = %d, want only the one that reached the reconciler", len(rc.resolves))
	}
}

func TestResolveSucceedsWithTheStaffActor(t *testing.T) {
	e := newEnv(t)
	_, rc := withNotices(e)
	n := reviewNotice(noticeA, t0)
	n.State, n.OrderID, n.DecidedAt = paymentnotice.StateResolved, orderA, t0
	rc.notice = n
	rec := e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+strings.ToUpper(orderA)+`","reason":" matched by hand "}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"resolved"`) || !strings.Contains(rec.Body.String(), `"order_id":"`+orderA+`"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "candidates") {
		t.Errorf("resolve response has candidates: %s", rec.Body)
	}
	if len(rc.resolves) != 1 {
		t.Fatalf("resolves = %+v", rc.resolves)
	}
	c := rc.resolves[0]
	if c.actor.ID != billID || c.actor.Label != billMail || c.actor.Role != "billing" || c.noticeID != noticeA || c.orderID != orderA || c.why != "matched by hand" {
		t.Errorf("call = %+v", c)
	}
}

func TestResolveClosedNoticeIs409WithState(t *testing.T) {
	e := newEnv(t)
	_, rc := withNotices(e)
	rc.err = &billing.NoticeClosedError{State: paymentnotice.StateApplied}
	rec := e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+orderA+`","reason":"x"}`)
	wantError(t, rec, http.StatusConflict, "payment_notice_closed")
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["state"] != "applied" {
		t.Errorf("body = %s, want state applied", rec.Body)
	}
	rec = e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/dismiss", `{"reason":"x"}`)
	wantError(t, rec, http.StatusConflict, "payment_notice_closed")
	if !strings.Contains(rec.Body.String(), `"state":"applied"`) {
		t.Errorf("dismiss body = %s", rec.Body)
	}
}

func TestResolveOrderNotPendingIs409(t *testing.T) {
	e := newEnv(t)
	_, rc := withNotices(e)
	rc.err = billing.ErrOrderNotPending
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+orderA+`","reason":"x"}`), http.StatusConflict, "order_not_pending")
	rc.err = billing.ErrNoticeConflict
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+orderA+`","reason":"x"}`), http.StatusConflict, "payment_notice_conflict")
	rc.err = paymentnotice.ErrNotFound
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+orderA+`","reason":"x"}`), http.StatusNotFound, "not_found")
	rc.err = billing.ErrOrderNotFound
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/resolve", `{"order_id":"`+orderA+`","reason":"x"}`), http.StatusNotFound, "order_not_found")
}

func TestDismissNeedsReason(t *testing.T) {
	e := newEnv(t)
	_, rc := withNotices(e)
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/dismiss", `{}`), http.StatusBadRequest, "reason_required")
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/dismiss", `{"reason":" "}`), http.StatusBadRequest, "reason_required")
	if len(rc.dismiss) != 0 {
		t.Fatalf("dismiss reached the reconciler: %+v", rc.dismiss)
	}
	rc.err = paymentnotice.ErrNotFound
	wantError(t, e.do(billID, "POST", "/api/platform/payment-notices/"+noticeA+"/dismiss", `{"reason":"refunded"}`), http.StatusNotFound, "not_found")

	rc.err = nil
	n := reviewNotice(noticeA, t0)
	n.State = paymentnotice.StateDismissed
	rc.notice = n
	rec := e.do(adminID, "POST", "/api/platform/payment-notices/"+noticeA+"/dismiss", `{"reason":"refunded"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"state":"dismissed"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if c := rc.dismiss[len(rc.dismiss)-1]; c.actor.ID != adminID || c.actor.Label != adminMail || c.actor.Role != "system_admin" || c.why != "refunded" {
		t.Errorf("call = %+v", c)
	}
}

// Support is refused by the staff middleware with the 404 of an unknown route,
// as on the order routes; nothing reaches the reader or the reconciler.
func TestSupportIsForbiddenOnPaymentNoticeRoutes(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"GET", "/api/platform/payment-notices", ""},
		{"GET", "/api/platform/payment-notices/" + noticeA, ""},
		{"POST", "/api/platform/payment-notices/" + noticeA + "/resolve", `{"order_id":"` + orderA + `","reason":"r"}`},
		{"POST", "/api/platform/payment-notices/" + noticeA + "/dismiss", `{"reason":"r"}`},
		{"GET", "/api/platform/einvoices", ""},
	}
	for _, rt := range routes {
		e := newEnv(t)
		nr, rc := withNotices(e)
		nr.notices = []paymentnotice.Notice{reviewNotice(noticeA, t0)}
		if rec := e.do(suppID, rt.method, rt.path, rt.body); rec.Code != http.StatusNotFound {
			t.Errorf("support %s %s = %d %s, want the 404 of an unknown route", rt.method, rt.path, rec.Code, rec.Body)
		}
		if got := e.do("", rt.method, rt.path, rt.body); got.Code != http.StatusNotFound {
			t.Errorf("anonymous %s %s = %d, want 404", rt.method, rt.path, got.Code)
		}
		if len(rc.resolves)+len(rc.dismiss) != 0 || nr.lastLimit != 0 || nr.lastInvLimit != 0 || nr.lastCandLimit != 0 {
			t.Errorf("support %s %s reached the handler", rt.method, rt.path)
		}
	}
}

func TestNoticeWritesNeedTheConsoleOrigin(t *testing.T) {
	e := newEnv(t)
	_, rc := withNotices(e)
	for _, p := range []string{"/resolve", "/dismiss"} {
		got := e.doWith(billID, "POST", "/api/platform/payment-notices/"+noticeA+p, `{"order_id":"`+orderA+`","reason":"r"}`, "https://evil.example")
		if got.Code == http.StatusOK {
			t.Errorf("%s with a foreign origin = 200", p)
		}
	}
	if len(rc.resolves)+len(rc.dismiss) != 0 {
		t.Fatal("a cross-origin request reached the reconciler")
	}
}
