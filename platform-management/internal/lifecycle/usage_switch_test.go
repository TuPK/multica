package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// Database tests of AC23, second half (spec 7b.1, rule 7): a switch from a seat
// plan to a usage plan is stored while the paid period runs and applies at the
// end of it, with no renewal order.

type noCache struct{}

func (noCache) Invalidate(string) {}
func (noCache) InvalidateAll()    {}

type switchFixture struct {
	*dbFixture
	usagePlan string
	end       time.Time
	changes   *subscription.PlanChanges
}

// newSwitchFixture is a workspace on the seat plan "pro" for 5 seats, paid from
// t0 to t0 + 30 days, with a usage plan to switch to.
func newSwitchFixture(t *testing.T) *switchFixture {
	t.Helper()
	f := &switchFixture{dbFixture: newDBFixture(t), end: t0.Add(30 * day)}
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ('Switch ' || md5(random()::text), 'switch-' || md5(random()::text))
		RETURNING id::text`).Scan(&f.ws); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT 'payg-sw-' || substr(md5(random()::text), 1, 10)`).Scan(&f.usagePlan); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO plan (key, name, currency, billing_model) VALUES ($1, 'Pay as you go (test)', 'vnd', 'usage')`, f.usagePlan); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.pool.Exec(ctx, `DELETE FROM billing_order WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, f.usagePlan)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, f.ws)
	})
	f.seed(subscription.Subscription{
		PlanKey: "pro", Status: subscription.StatusActive, Seats: 5, Interval: "month",
		CurrentPeriodStart: tp(t0), CurrentPeriodEnd: tp(f.end), Version: 3,
	})
	if _, err := f.pool.Exec(ctx, `UPDATE workspace_subscription SET unit_price = 100000 WHERE workspace_id = $1`, f.ws); err != nil {
		t.Fatal(err)
	}
	f.changes = subscription.NewPlanChanges(subscription.NewPgxRunner(f.pool), noCache{},
		subscription.NewPgxPendingOrders(f.pool), func() time.Time { return t0.Add(10 * day) })
	return f
}

func (f *switchFixture) schedule() subscription.Result {
	f.t.Helper()
	res, err := f.changes.Schedule(context.Background(), subscription.ScheduleChange{
		WorkspaceID: f.ws, PlanKey: f.usagePlan, Interval: "month", Seats: 0,
	}, subscription.Actor{ID: staffID})
	if err != nil {
		f.t.Fatalf("Schedule: %v", err)
	}
	return res
}

func (f *switchFixture) run(now time.Time) {
	f.t.Helper()
	if _, err := f.svc.Run(context.Background(), now); err != nil {
		f.t.Fatalf("Run(%s): %v", now.Format(time.RFC3339), err)
	}
}

type switchRow struct {
	plan, status, interval string
	seats                  int
	version                int64
	pending                bool
	unitPrice              *int64
	periodStart, periodEnd *time.Time
}

func (f *switchFixture) row() switchRow {
	f.t.Helper()
	var r switchRow
	if err := f.pool.QueryRow(context.Background(),
		`SELECT plan_key, status, interval, seats, version, pending_change IS NOT NULL, unit_price,
		        current_period_start, current_period_end
		 FROM workspace_subscription WHERE workspace_id = $1`, f.ws).
		Scan(&r.plan, &r.status, &r.interval, &r.seats, &r.version, &r.pending, &r.unitPrice, &r.periodStart, &r.periodEnd); err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f *switchFixture) orders() int {
	return f.count(`SELECT count(*) FROM billing_order WHERE workspace_id = $1`)
}

func (f *switchFixture) audits() int {
	return f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1`)
}

func TestSeatToUsageSwitchIsStoredAndAppliedAtPeriodEnd(t *testing.T) {
	f := newSwitchFixture(t)

	// (a) The request stores pending_change and changes nothing else.
	if res := f.schedule(); !res.Changed {
		t.Fatal("the request stored nothing")
	}
	got := f.row()
	if !got.pending || got.plan != "pro" || got.seats != 5 || got.status != "active" || got.interval != "month" ||
		got.unitPrice == nil || *got.unitPrice != 100000 || got.version != 4 {
		t.Fatalf("after the request: %+v, want the seat plan kept with a pending change at version 4", got)
	}
	if f.orders() != 0 || f.audits() != 1 {
		t.Fatalf("orders = %d, audit rows = %d; want 0 and 1", f.orders(), f.audits())
	}

	// Repeating the request writes nothing.
	if res := f.schedule(); res.Changed || f.row().version != 4 || f.audits() != 1 {
		t.Fatalf("a repeat changed the row: %+v, version %d, audit rows %d", res, f.row().version, f.audits())
	}

	// (b) Before the period end the seat plan stays in force.
	f.run(f.end.Add(-time.Hour))
	if got := f.row(); got.plan != "pro" || got.seats != 5 || got.status != "active" || !got.pending || got.version != 4 {
		t.Fatalf("before the period end: %+v", got)
	}

	// (c) At the period end the usage plan is applied: no renewal order, still
	// active, no seats, pending_change cleared, a month-long usage period.
	f.run(f.end)
	got = f.row()
	if got.plan != f.usagePlan || got.status != "active" || got.seats != 0 || got.pending || got.unitPrice != nil || got.interval != "month" {
		t.Fatalf("at the period end: %+v, want the usage plan, active, 0 seats, nothing pending, no unit price", got)
	}
	if got.periodEnd == nil || !got.periodEnd.After(f.end) || got.periodStart == nil || !got.periodStart.Equal(f.end) {
		t.Fatalf("usage period = %v to %v, want one that starts at %s", got.periodStart, got.periodEnd, f.end)
	}
	if f.orders() != 0 {
		t.Fatalf("%d orders were written; the switch has no renewal order", f.orders())
	}
	if f.audits() != 2 {
		t.Fatalf("audit rows = %d, want 2 (scheduled, applied)", f.audits())
	}

	// (d) A second run, now and later, changes nothing more.
	after := f.row()
	f.run(f.end)
	f.run(f.end.Add(40 * day))
	if again := f.row(); again.version != after.version || again.plan != after.plan || again.status != "active" || f.audits() != 2 || f.orders() != 0 {
		t.Fatalf("a later run changed the row: %+v (audit rows %d, orders %d)", again, f.audits(), f.orders())
	}
}

// Withdrawing the pending change before the period end keeps the seat plan, so
// the period lapses as it would have without the request.
func TestCanceledSeatToUsageSwitchDoesNotApply(t *testing.T) {
	f := newSwitchFixture(t)
	f.schedule()
	if _, err := f.changes.CancelScheduled(context.Background(), f.ws, subscription.Actor{ID: staffID}); err != nil {
		t.Fatal(err)
	}
	f.run(f.end)
	if got := f.row(); got.plan != "pro" || got.status != StatusPastDue || got.pending {
		t.Fatalf("after the withdrawal: %+v, want the seat plan lapsed to past_due", got)
	}
}

// A subscription that is canceled at the period end goes to free; the pending
// change has no effect until it is resumed (spec 7b.1, rule 8).
func TestSeatToUsageSwitchIsIgnoredWhenTheSubscriptionIsCanceled(t *testing.T) {
	f := newSwitchFixture(t)
	f.schedule()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE workspace_subscription SET cancel_at_period_end = true WHERE workspace_id = $1`, f.ws); err != nil {
		t.Fatal(err)
	}
	f.run(f.end)
	if got := f.row(); got.plan != "free" || got.status != StatusFree {
		t.Fatalf("canceled subscription: %+v, want free", got)
	}
}
