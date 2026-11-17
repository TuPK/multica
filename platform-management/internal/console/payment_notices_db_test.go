package console

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Database tests for the payment notice routes. They share the database with
// other packages, so every assertion filters on the provider name or the ids
// of this test's own rows.

type noticeFixture struct {
	*orderFixture
	provider string
}

func newNoticeFixture(t *testing.T) *noticeFixture {
	t.Helper()
	o := newOrderFixture(t)
	f := &noticeFixture{orderFixture: o, provider: "test-" + randomHex(t)}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	payments, err := billing.New(billing.Deps{
		Runner: billing.NewPgxRunner(o.pool), Provider: billing.ProviderManual,
		Workspaces: workspacestate.New(nil, o.cache), Entitlements: noopEntitlements{}, Log: quiet,
	})
	if err != nil {
		t.Fatal(err)
	}
	o.h.(*Handler).Reconciler = billing.NewReconciler(payments, billing.NewPgxReconcileStore(o.pool))
	t.Cleanup(func() {
		ctx := context.Background()
		o.pool.Exec(ctx, `DELETE FROM audit_event WHERE detail->>'provider' = $1`, f.provider)
		o.pool.Exec(ctx, `DELETE FROM payment_notice WHERE provider = $1`, f.provider)
	})
	return f
}

// seedNotice inserts a notice of this test's provider and returns its id.
func (f *noticeFixture) seedNotice(event, state, code string, amount int64, receivedAt time.Time) string {
	f.t.Helper()
	var id string
	err := f.pool.QueryRow(context.Background(), `
		INSERT INTO payment_notice (provider, provider_event_id, state, review_reason, transfer_code, amount, currency, body_sha256, received_at)
		VALUES ($1, $2, $3, CASE WHEN $3 = 'needs_review' THEN 'amount_mismatch' ELSE '' END, $4, $5, 'VND', 'aa', $6)
		RETURNING id::text`, f.provider, event, state, code, amount, receivedAt).Scan(&id)
	if err != nil {
		f.t.Fatalf("seed notice: %v", err)
	}
	return id
}

func (f *noticeFixture) listed(rec *httptest.ResponseRecorder) []map[string]any {
	f.t.Helper()
	var body struct {
		Notices []map[string]any `json:"notices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		f.t.Fatalf("body %s: %v", rec.Body, err)
	}
	var mine []map[string]any
	for _, n := range body.Notices {
		if n["provider"] == f.provider {
			mine = append(mine, n)
		}
	}
	return mine
}

func TestNoticeListFiltersByState(t *testing.T) {
	f := newNoticeFixture(t)
	billingID, _ := f.staffUser(staff.RoleBilling)
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := f.seedNotice("e2", "needs_review", "MULNOTICE02", 100, old.Add(time.Hour))
	older := f.seedNotice("e1", "needs_review", "MULNOTICE01", 100, old)
	applied := f.seedNotice("e3", "applied", "MULNOTICE03", 100, future)
	dismissed := f.seedNotice("e4", "dismissed", "MULNOTICE04", 100, future.Add(time.Hour))

	rec := f.do(billingID, "GET", "/api/platform/payment-notices?limit=200", "")
	wantStatus(t, rec, 200)
	mine := f.listed(rec)
	if len(mine) != 2 || mine[0]["id"] != older || mine[1]["id"] != newer {
		t.Fatalf("default list = %+v, want the two needs_review notices, oldest first", mine)
	}
	if mine[0]["state"] != "needs_review" || mine[0]["review_reason"] != "amount_mismatch" || mine[0]["order_id"] != "" || mine[0]["currency"] != "VND" {
		t.Errorf("item = %+v", mine[0])
	}
	if _, ok := mine[0]["decided_at"]; ok && mine[0]["decided_at"] != nil {
		t.Errorf("an undecided notice has decided_at %v", mine[0]["decided_at"])
	}

	mine = f.listed(f.do(billingID, "GET", "/api/platform/payment-notices?state=applied&limit=200", ""))
	if len(mine) != 1 || mine[0]["id"] != applied {
		t.Errorf("applied list = %+v", mine)
	}
	mine = f.listed(f.do(billingID, "GET", "/api/platform/payment-notices?state=dismissed&limit=200", ""))
	if len(mine) != 1 || mine[0]["id"] != dismissed {
		t.Errorf("dismissed list = %+v", mine)
	}
	if mine = f.listed(f.do(billingID, "GET", "/api/platform/payment-notices?state=resolved&limit=200", "")); len(mine) != 0 {
		t.Errorf("resolved list = %+v", mine)
	}

	// The detail names pending orders that match by code or by total.
	byCode := f.seedOrder("pending", time.Hour, 24*time.Hour)
	if _, err := f.pool.Exec(context.Background(), `UPDATE billing_order SET transfer_code = 'MULNOTICE01' WHERE id = $1`, byCode); err != nil {
		t.Fatal(err)
	}
	byAmount := f.seedOrder("pending", time.Hour, 24*time.Hour) // total 660000
	paid := f.seedOrder("paid", time.Hour, 24*time.Hour)
	expired := f.seedOrder("pending", time.Hour, -time.Hour)
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE payment_notice SET amount = 660000 WHERE id = $1`, older); err != nil {
		t.Fatal(err)
	}
	rec = f.do(billingID, "GET", "/api/platform/payment-notices/"+older, "")
	wantStatus(t, rec, 200)
	var detail struct {
		Candidates []struct {
			OrderID string `json:"order_id"`
			Match   string `json:"match"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range detail.Candidates {
		got[c.OrderID] = c.Match
	}
	if got[byCode] != "code" || got[byAmount] != "amount" {
		t.Errorf("candidates = %v, want %s by code and %s by amount", got, byCode, byAmount)
	}
	if _, ok := got[paid]; ok {
		t.Error("a paid order is a candidate")
	}
	if _, ok := got[expired]; ok {
		t.Error("an expired order is a candidate")
	}
}

func TestResolveThroughRouteWritesAuditWithBothAmounts(t *testing.T) {
	f := newNoticeFixture(t)
	billingID, billingEmail := f.staffUser(staff.RoleBilling)
	f.seedSubscription(4)
	orderID := f.seedOrder("pending", time.Hour, 24*time.Hour) // total 660000
	noticeID := f.seedNotice("resolve-1", "needs_review", "MULNOTRESOLVE", 650000, time.Now().UTC())

	rec := f.do(billingID, "POST", "/api/platform/payment-notices/"+noticeID+"/resolve",
		`{"order_id":"`+orderID+`","reason":"customer sent 10000 less; accepted"}`)
	wantStatus(t, rec, 200)
	got := decodeJSON(t, rec)
	if got["state"] != "resolved" || got["order_id"] != orderID || got["id"] != noticeID {
		t.Errorf("body = %s", rec.Body)
	}
	if f.orderStatus(orderID) != "paid" {
		t.Errorf("order status = %s, want paid", f.orderStatus(orderID))
	}

	rows := f.staffRows("payment.notice_resolved", billingID)
	var mine []auditRow
	for _, r := range rows {
		if r.targetID == noticeID {
			mine = append(mine, r)
		}
	}
	if len(mine) != 1 {
		t.Fatalf("payment.notice_resolved rows for the notice = %d, want 1", len(mine))
	}
	r := mine[0]
	if r.actorType != "staff" || r.actorLabel != billingEmail || r.workspaceID != f.ws {
		t.Errorf("row = %+v", r)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(r.detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["notice_amount"] != float64(650000) || detail["order_total"] != float64(660000) || detail["order_id"] != orderID {
		t.Errorf("detail = %s, want notice_amount 650000 and order_total 660000", r.detail)
	}

	// The same resolve again changes nothing and answers 200.
	wantStatus(t, f.do(billingID, "POST", "/api/platform/payment-notices/"+noticeID+"/resolve",
		`{"order_id":"`+orderID+`","reason":"again"}`), 200)
	if n := f.count(`SELECT count(*) FROM audit_event WHERE action = 'payment.notice_resolved' AND target_id = $1`, noticeID); n != 1 {
		t.Errorf("audit rows after the repeat = %d, want 1", n)
	}

	// The closed notice cannot be dismissed.
	rec = f.do(billingID, "POST", "/api/platform/payment-notices/"+noticeID+"/dismiss", `{"reason":"x"}`)
	wantStatus(t, rec, 409)
	if got := decodeJSON(t, rec); got["code"] != "payment_notice_closed" || got["state"] != "resolved" {
		t.Errorf("dismiss body = %s", rec.Body)
	}
}
