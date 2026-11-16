package billing

import (
	"context"
	"testing"
	"time"
)

// Database tests of the pay as you go activation (spec 7b.3, rule 6). They use
// the billing fixture of db_test.go: a fresh workspace per test, removed after.

func (f *dbFixture) usageActivation(key string) UsageActivation {
	return UsageActivation{WorkspaceID: f.ws, PlanKey: "payg", CreatedBy: f.user, IdempotencyKey: key}
}

func TestDBUsageActivationWritesAnActiveSubscriptionAndAPaidZeroOrder(t *testing.T) {
	f := newDBFixture(t)
	now := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	s := f.service(nil, func() time.Time { return now })

	o, created, err := s.ActivateUsage(context.Background(), f.usageActivation("act-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the first activation reports created = false")
	}
	if o.Kind != KindSubscribe || o.Status != StatusPaid || o.Amount != 0 || o.VATAmount != 0 || o.Seats != 0 || o.PlanKey != "payg" {
		t.Fatalf("order = %+v, want a paid subscribe order for 0 VND on payg with 0 seats", o)
	}

	var status, plan, interval string
	var seats int
	var start, end time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT status, plan_key, seats, interval, current_period_start, current_period_end
		FROM workspace_subscription WHERE workspace_id = $1`, f.ws).Scan(&status, &plan, &seats, &interval, &start, &end); err != nil {
		t.Fatal(err)
	}
	if status != "active" || plan != "payg" || seats != 0 || interval != "month" {
		t.Fatalf("subscription = %s %s seats %d %s, want active payg with 0 seats on a month", status, plan, seats, interval)
	}
	if !start.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) || !end.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("period = [%s, %s), want the UTC calendar month of November", start, end)
	}
	if n := f.count(`SELECT count(*) FROM billing_order WHERE workspace_id = $1`, f.ws); n != 1 {
		t.Fatalf("orders = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'subscription.changed' AND actor_type = 'member'`, f.ws); n != 1 {
		t.Fatalf("subscription.changed audit rows by a member = %d, want 1", n)
	}
}

func TestDBRepeatedUsageActivationWithTheSameKeyChangesNothing(t *testing.T) {
	f := newDBFixture(t)
	now := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	s := f.service(nil, func() time.Time { return now })

	first, _, err := s.ActivateUsage(context.Background(), f.usageActivation("act-2"))
	if err != nil {
		t.Fatal(err)
	}
	again, created, err := s.ActivateUsage(context.Background(), f.usageActivation("act-2"))
	if err != nil {
		t.Fatal(err)
	}
	if created || again.ID != first.ID {
		t.Fatalf("repeat = order %s created %v, want the first order %s and created false", again.ID, created, first.ID)
	}
	if version, _ := f.subscriptionVersion(); version != 1 {
		t.Fatalf("subscription version = %d after a repeat, want 1", version)
	}
	if n := f.count(`SELECT count(*) FROM billing_order WHERE workspace_id = $1`, f.ws); n != 1 {
		t.Fatalf("orders = %d after a repeat, want 1", n)
	}
}
