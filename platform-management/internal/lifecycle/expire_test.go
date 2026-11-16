package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
)

const orderID = "11111111-1111-1111-1111-111111111111"

func pendingOrder(id, ws string, expiresAt time.Time) billing.Order {
	return billing.Order{
		ID: id, WorkspaceID: ws, Kind: billing.KindChangePlan, PlanKey: "pro", Interval: "month", Seats: 5,
		Amount: 100000, VATAmount: 10000, Currency: "VND", Status: billing.StatusPending,
		TransferCode: "MUL123", ExpiresAt: expiresAt, CreatedAt: expiresAt.Add(-billing.OrderLifetime),
	}
}

func newExpirer(t *testing.T, f *fakeStore) *OrderExpirer {
	t.Helper()
	x, err := NewOrderExpirer(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func (f *fakeStore) order(id string) billing.Order {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.orders[id]
}

func (f *fakeStore) auditCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.audits)
}

func expireRun(t *testing.T, x *OrderExpirer, now time.Time) int {
	t.Helper()
	n, err := x.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("Run(%s): %v", now.Format(time.RFC3339), err)
	}
	return n
}

// An order is expired at expires_at itself, and not one second before.
func TestOrderExpiresAtExactlyExpiresAtAndNotBefore(t *testing.T) {
	f := newFakeStore()
	exp := t0.Add(7 * day)
	f.orders[orderID] = pendingOrder(orderID, wsID(1), exp)
	x := newExpirer(t, f)

	if n := expireRun(t, x, exp.Add(-time.Second)); n != 0 {
		t.Fatalf("expired %d orders one second before expires_at, want 0", n)
	}
	if got := f.order(orderID).Status; got != billing.StatusPending {
		t.Fatalf("status one second before = %q, want pending", got)
	}
	if f.auditCount() != 0 {
		t.Fatalf("audit rows one second before = %d, want 0", f.auditCount())
	}

	if n := expireRun(t, x, exp); n != 1 {
		t.Fatalf("expired %d orders at expires_at, want 1", n)
	}
	if got := f.order(orderID).Status; got != billing.StatusExpired {
		t.Fatalf("status at expires_at = %q, want expired", got)
	}
}

// A second run changes nothing: no second status write and no second audit row.
func TestOrderExpiryTwiceChangesNothingTheSecondTime(t *testing.T) {
	f := newFakeStore()
	exp := t0.Add(7 * day)
	f.orders[orderID] = pendingOrder(orderID, wsID(1), exp)
	x := newExpirer(t, f)

	if n := expireRun(t, x, exp.Add(time.Hour)); n != 1 {
		t.Fatalf("first run expired %d, want 1", n)
	}
	before := f.order(orderID)
	if n := expireRun(t, x, exp.Add(2*time.Hour)); n != 0 {
		t.Fatalf("second run expired %d, want 0", n)
	}
	if got := f.order(orderID); got.Status != before.Status || got.PaidAt != before.PaidAt {
		t.Fatalf("the order changed on the second run: %+v, was %+v", got, before)
	}
	if f.auditCount() != 1 {
		t.Fatalf("audit rows after two runs = %d, want 1", f.auditCount())
	}
}

// A paid order is never expired, even when its expires_at has passed.
func TestOrderExpiryNeverTouchesAPaidOrder(t *testing.T) {
	f := newFakeStore()
	exp := t0.Add(7 * day)
	paid := pendingOrder(orderID, wsID(1), exp)
	paid.Status = billing.StatusPaid
	f.orders[orderID] = paid
	x := newExpirer(t, f)

	if n := expireRun(t, x, exp.Add(30*day)); n != 0 {
		t.Fatalf("expired %d orders, want 0", n)
	}
	if got := f.order(orderID).Status; got != billing.StatusPaid {
		t.Fatalf("status = %q, want paid", got)
	}
	if f.auditCount() != 0 {
		t.Fatalf("audit rows = %d, want 0", f.auditCount())
	}
}

// The step writes exactly one billing.order_expired row: actor system, target
// the order, and a detail free of a planted secret.
func TestOrderExpiryWritesOneAuditRowWithSystemActor(t *testing.T) {
	f := newFakeStore()
	exp := t0.Add(7 * day)
	o := pendingOrder(orderID, wsID(1), exp)
	// The order carries a planted secret in fields the audit row does not copy.
	o.ProviderRef, o.Buyer = "sk_live_PLANTED_SECRET", json.RawMessage(`{"token":"PLANTED_SECRET"}`)
	f.orders[orderID] = o
	x := newExpirer(t, f)
	expireRun(t, x, exp)

	if len(f.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(f.audits))
	}
	e := f.audits[0]
	if e.Action != "billing.order_expired" {
		t.Errorf("action = %q, want billing.order_expired", e.Action)
	}
	if e.ActorType != audit.ActorSystem || e.ActorID != "" {
		t.Errorf("actor = %q/%q, want system with no id", e.ActorType, e.ActorID)
	}
	if e.TargetType != "billing_order" || e.TargetID != orderID {
		t.Errorf("target = %q/%q, want billing_order/%s", e.TargetType, e.TargetID, orderID)
	}
	if e.WorkspaceID != wsID(1) {
		t.Errorf("workspace = %q, want %q", e.WorkspaceID, wsID(1))
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "PLANTED_SECRET") {
		t.Errorf("a planted secret reached the audit entry: %s", raw)
	}
}

// A failed audit write rolls the status change back, so the order stays
// pending and the next run expires it.
func TestOrderExpiryRollsBackWhenTheAuditWriteFails(t *testing.T) {
	f := newFakeStore()
	exp := t0.Add(7 * day)
	f.orders[orderID] = pendingOrder(orderID, wsID(1), exp)
	x := newExpirer(t, f)

	f.failAudit = true
	n, err := x.Run(context.Background(), exp)
	if err == nil {
		t.Fatal("Run returned no error for a failed audit write")
	}
	if n != 0 {
		t.Fatalf("expired = %d, want 0", n)
	}
	if got := f.order(orderID).Status; got != billing.StatusPending {
		t.Fatalf("status after a failed audit write = %q, want pending (rolled back)", got)
	}
	if f.auditCount() != 0 {
		t.Fatalf("audit rows = %d, want 0", f.auditCount())
	}

	f.failAudit = false
	if n := expireRun(t, x, exp); n != 1 {
		t.Fatalf("retry expired %d, want 1", n)
	}
	if f.auditCount() != 1 {
		t.Fatalf("audit rows after the retry = %d, want 1", f.auditCount())
	}
}
