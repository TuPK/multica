package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// catalogueFor returns the catalogue's plans by key. The seat prices are pro
// 149000 a month and 1490000 a year, unlimited 249000 and 2490000.
func catalogueFor(t *testing.T) map[string]plan.Plan {
	t.Helper()
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]plan.Plan{}
	for _, p := range all {
		out[p.Key] = p
	}
	return out
}

// Plan serves the catalogue to the in-memory transaction.
func (t *memTx) Plan(_ context.Context, key string) (plan.Plan, bool, error) {
	all, err := plan.Catalogue()
	if err != nil {
		return plan.Plan{}, false, err
	}
	for _, p := range all {
		if p.Key == key {
			return p, true, nil
		}
	}
	return plan.Plan{}, false, nil
}

// PendingOrder returns the workspace's pending order, as the database would.
func (t *memTx) PendingOrder(_ context.Context, ws string) (Order, bool, error) {
	for _, o := range t.m.orders {
		if o.WorkspaceID == ws && o.Status == StatusPending {
			return o, true, nil
		}
	}
	return Order{}, false, nil
}

// proSeats3 is a pro workspace of 3 seats, paying 149000 a seat for March 2025.
// The clock is 1 March 2025 10:00, so the whole of March is still left in 30.58
// days: the upgrade to unlimited (249000 a seat) is 300000, charged in full.
func proSeats3(m *memory) {
	start, end := date(2025, 3, 1), date(2025, 4, 1)
	m.subs[tWS] = subscription.Subscription{
		WorkspaceID: tWS, PlanKey: "pro", Status: subscription.StatusActive, Seats: 3, Interval: IntervalMonth,
		CurrentPeriodStart: &start, CurrentPeriodEnd: &end, UnitPrice: i64(149000), Version: 4,
		LimitOverrides: plan.Limits{},
	}
}

func upgradeOrder(key string, accepted int64) ChangeOrder {
	return ChangeOrder{
		WorkspaceID: tWS, Request: subscription.Request{PlanKey: "unlimited", Interval: IntervalMonth},
		AcceptedAmount: accepted, VATRate: DefaultVATRate, IdempotencyKey: key, CreatedBy: tUser,
	}
}

func TestChangeOrderRepeatedKeyReturnsTheFirstOrder(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	s, _ := newTestService(t, m)
	first, created, err := s.CreateChangeOrder(context.Background(), upgradeOrder("up-1", 300000))
	if err != nil || !created {
		t.Fatalf("first call: created %v, err %v", created, err)
	}
	second, created, err := s.CreateChangeOrder(context.Background(), upgradeOrder("up-1", 300000))
	if err != nil {
		t.Fatal(err)
	}
	if created || second.ID != first.ID {
		t.Fatalf("repeat: created %v, order %s, want the first order %s", created, second.ID, first.ID)
	}
	if len(m.orders) != 1 || len(actions(m)) != 1 {
		t.Fatalf("orders %d, audit rows %d; want one of each", len(m.orders), len(actions(m)))
	}
	if first.Kind != KindChangePlan || first.PlanKey != "unlimited" || first.Seats != 3 || first.Amount != 300000 || first.VATAmount != 30000 {
		t.Fatalf("order = %+v", first)
	}
}

func TestChangeOrderRefusesAnotherPendingOrder(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	s, _ := newTestService(t, m)
	pending := pendingOrder(t, m, s, testDraft()) // a pending subscribe order under key k1
	_, _, err := s.CreateChangeOrder(context.Background(), upgradeOrder("up-2", 300000))
	var e *subscription.Error
	if !errors.As(err, &e) || e.Status != 409 || e.Code != "order_pending" {
		t.Fatalf("err = %v, want 409 order_pending", err)
	}
	if e.Detail["order_id"] != pending.ID {
		t.Fatalf("detail = %v, want order_id %s", e.Detail, pending.ID)
	}
	if len(m.orders) != 1 {
		t.Fatalf("orders = %d, want only the first", len(m.orders))
	}
}

func TestChangeOrderRefusesAMismatchedAcceptedAmount(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	s, _ := newTestService(t, m)
	_, _, err := s.CreateChangeOrder(context.Background(), upgradeOrder("up-3", 299999))
	var e *subscription.Error
	if !errors.As(err, &e) || e.Status != 409 || e.Code != "quote_changed" {
		t.Fatalf("err = %v, want 409 quote_changed", err)
	}
	if len(m.orders) != 0 || len(actions(m)) != 0 {
		t.Fatalf("a refused quote wrote: orders %d, audit %v", len(m.orders), actions(m))
	}
}

func TestChangeOrderRefusesAStaleVersion(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	s, _ := newTestService(t, m)
	c := upgradeOrder("up-4", 300000)
	c.ExpectedVersion = 3
	_, _, err := s.CreateChangeOrder(context.Background(), c)
	var e *subscription.Error
	if !errors.As(err, &e) || e.Code != "quote_changed" {
		t.Fatalf("err = %v, want quote_changed", err)
	}
}

func TestApplyingAChangeOrderTwiceChangesTheSubscriptionOnce(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	s, _ := newTestService(t, m)
	o, _, err := s.CreateChangeOrder(context.Background(), upgradeOrder("up-5", 300000))
	if err != nil {
		t.Fatal(err)
	}
	conf := Confirmation{OrderID: o.ID, ConfirmedBy: tStaff, ActorLabel: "billing@example.com"}
	first, err := s.ApplyPayment(context.Background(), conf)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ApplyPayment(context.Background(), conf)
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyPaid || second.Order.ID != o.ID {
		t.Fatalf("second apply = %+v, want AlreadyPaid", second)
	}
	got := m.subs[tWS]
	if got.Version != first.Subscription.Version || got.Version != 5 {
		t.Fatalf("version = %d after two payments, want 5 (one change)", got.Version)
	}
	if got.PlanKey != "unlimited" || got.Seats != 3 || got.UnitPrice == nil || *got.UnitPrice != 249000 {
		t.Fatalf("subscription after the upgrade = %+v", got)
	}
}

func TestApplyPaymentSetsUnitPrice(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o := pendingOrder(t, m, s, testDraft()) // subscribe: pro, 3 seats, month
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err != nil {
		t.Fatal(err)
	}
	got := m.subs[tWS]
	if got.UnitPrice == nil || *got.UnitPrice != 149000 {
		t.Fatalf("unit price = %v, want 149000 (the seat price paid)", got.UnitPrice)
	}
}

// Spec 7b.1 rule 6: a renewal is priced from the pending change, and applying
// it clears the pending change.
func TestRenewalIsPricedFromAndClearsThePendingChange(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	cur := m.subs[tWS]
	cur.PendingChange = &subscription.PendingChange{PlanKey: "pro", Seats: 2, Interval: IntervalYear, RequestedBy: tUser}
	m.subs[tWS] = cur
	s, _ := newTestService(t, m)

	pro := catalogueFor(t)["pro"]
	start := date(2025, 4, 1)
	d, err := RenewalDraft(m.subs[tWS], pro, start)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != KindRenew || d.PlanKey != "pro" || d.Interval != IntervalYear || d.Seats != 2 {
		t.Fatalf("draft = %+v, want a renew of pro, year, 2 seats", d)
	}
	if d.Amount != 2*1490000 {
		t.Fatalf("amount = %d, want the year price of 2 seats", d.Amount)
	}
	o, _, err := s.CreateOrder(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err != nil {
		t.Fatal(err)
	}
	got := m.subs[tWS]
	if got.PendingChange != nil {
		t.Fatalf("pending change = %+v after the renewal; want it cleared", got.PendingChange)
	}
	if got.Interval != IntervalYear || got.Seats != 2 || got.PlanKey != "pro" {
		t.Fatalf("subscription after the renewal = %+v", got)
	}
	if got.UnitPrice == nil || *got.UnitPrice != 1490000 {
		t.Fatalf("unit price = %v, want the year seat price", got.UnitPrice)
	}
}

func TestRenewalWithAMismatchedOrderIsRefused(t *testing.T) {
	m := newMemory()
	proSeats3(m)
	cur := m.subs[tWS]
	cur.PendingChange = &subscription.PendingChange{PlanKey: "pro", Seats: 2, Interval: IntervalYear, RequestedBy: tUser}
	m.subs[tWS] = cur
	s, _ := newTestService(t, m)
	start, end := date(2025, 4, 1), date(2026, 4, 1)
	o, _, err := s.CreateOrder(context.Background(), Draft{
		WorkspaceID: tWS, Kind: KindRenew, PlanKey: "pro", Interval: IntervalMonth, Seats: 3,
		Amount: 3 * 149000, Currency: "vnd", PeriodStart: &start, PeriodEnd: &end, IdempotencyKey: "ren-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err == nil {
		t.Fatal("a renewal that does not match the pending change was applied")
	}
}
