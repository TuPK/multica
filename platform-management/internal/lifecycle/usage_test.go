package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Database tests of the usage debt path (spec 7b.3, rule 13). The job reads
// every subscription of the shared database, so each test asserts on its own
// workspace.

// usageFixture is a workspace on a pay as you go plan with one unpaid usage
// invoice created at t0.
type usageFixture struct {
	*dbFixture
	planKey string
	orderID string
}

func newUsageFixture(t *testing.T, status string, graceUntil *time.Time) *usageFixture {
	t.Helper()
	f := &usageFixture{dbFixture: newDBFixture(t)}
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT 'payg-lc-' || substr(md5(random()::text), 1, 10)`).Scan(&f.planKey); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO plan (key, name, currency, billing_model) VALUES ($1, 'Pay as you go (test)', 'vnd', 'usage')`, f.planKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.pool.Exec(ctx, `DELETE FROM usage_charge WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(ctx, `DELETE FROM billing_order WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, f.planKey)
	})
	end := t0.AddDate(0, 1, 0)
	f.seed(subscription.Subscription{
		PlanKey: f.planKey, Status: status, Interval: "month", GraceUntil: graceUntil,
		CurrentPeriodStart: tp(t0), CurrentPeriodEnd: tp(end), Version: 3,
	})
	f.orderID = f.usageOrder(t0)
	return f
}

// usageOrder inserts a pending usage invoice created at createdAt, with one
// usage_charge row on it, and returns its id.
func (f *usageFixture) usageOrder(createdAt time.Time) string {
	f.t.Helper()
	ctx := context.Background()
	var id string
	if err := f.pool.QueryRow(ctx, `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount,
		    currency, status, provider, transfer_code, idempotency_key, buyer, period_start, period_end, created_at)
		VALUES (gen_random_uuid(), $1, 'usage', $2, 'month', 0, 50000, 5000, 'vnd', 'pending', 'manual',
		    'MUL' || upper(substr(md5(random()::text), 1, 8)), 'usage:' || md5(random()::text), '{}', $3, $4, $3)
		RETURNING id::text`, f.ws, f.planKey, createdAt, createdAt.AddDate(0, 1, 0)).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO usage_charge (workspace_id, day, meter, quantity, plan_key, unit_price, per, discount_pct, net_amount, order_id)
		VALUES ($1, $2, 'tasks_run', 1, $3, 1, 1, 0, 50000, $4)`, f.ws, createdAt.Format("2006-01-02"), f.planKey, id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *usageFixture) run(now time.Time) {
	f.t.Helper()
	if _, err := f.svc.Run(context.Background(), now); err != nil {
		f.t.Fatalf("Run(%s): %v", now.Format(time.RFC3339), err)
	}
}

func (f *usageFixture) state() (status string, grace *time.Time, version int64) {
	f.t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, grace_until, version FROM workspace_subscription WHERE workspace_id = $1`, f.ws).Scan(&status, &grace, &version); err != nil {
		f.t.Fatal(err)
	}
	return status, grace, version
}

func (f *usageFixture) workspaceState() (status, reason string, found bool) {
	f.t.Helper()
	err := f.pool.QueryRow(context.Background(),
		`SELECT status, reason FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&status, &reason)
	return status, reason, err == nil
}

func (f *usageFixture) orderStatus(id string) string {
	f.t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM billing_order WHERE id = $1`, id).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func TestUsageLifecycleOverdueToPastDueReadOnlyFree(t *testing.T) {
	f := newUsageFixture(t, subscription.StatusActive, nil)

	f.run(t0.Add(6*day + time.Hour))
	if status, _, _ := f.state(); status != subscription.StatusActive {
		t.Fatalf("status before the due day = %q, want active", status)
	}

	// Seven days after the invoice: past_due, with the grace counted from created_at.
	f.run(t0.Add(7 * day))
	status, grace, _ := f.state()
	if status != StatusPastDue || grace == nil || !grace.Equal(t0.Add(7*day+GracePeriod)) {
		t.Fatalf("after the due day: status %q, grace_until %v; want past_due until %s", status, grace, t0.Add(7*day+GracePeriod))
	}
	if ws, _, found := f.workspaceState(); found {
		t.Fatalf("workspace is %q after past_due, want it untouched", ws)
	}

	// The grace period ends: the workspace becomes read-only for billing.
	f.run(t0.Add(7*day + GracePeriod))
	if status, _, _ := f.state(); status != StatusReadOnly {
		t.Fatalf("status after the grace period = %q, want read_only", status)
	}
	if ws, reason, found := f.workspaceState(); !found || ws != string(workspacestate.ReadOnly) || reason != billing.ReasonBilling {
		t.Fatalf("workspace = %q, %q (found %v), want read_only with the reason billing", ws, reason, found)
	}

	// Thirty days after grace_until: free, and the workspace is active again.
	f.run(t0.Add(7*day + GracePeriod + ReadOnlyPeriod))
	if status, _, _ := f.state(); status != StatusFree {
		t.Fatalf("status after the read-only period = %q, want free", status)
	}
	if ws, reason, _ := f.workspaceState(); ws != string(workspacestate.Active) || reason != reasonFreePlan {
		t.Fatalf("workspace = %q, %q, want active with the reason %q", ws, reason, reasonFreePlan)
	}

	// Nothing is deleted: the invoice is still the debt, and its lines stay on it.
	if got := f.orderStatus(f.orderID); got != "pending" {
		t.Errorf("order status = %q, want pending", got)
	}
	if n := f.count(`SELECT count(*) FROM usage_charge WHERE workspace_id = $1 AND order_id IS NOT NULL`); n != 1 {
		t.Errorf("invoiced usage_charge rows = %d, want 1", n)
	}
}

func TestUsageLifecyclePaymentReturnsActive(t *testing.T) {
	grace := t0.Add(7*day + GracePeriod)
	for _, tc := range []struct {
		name        string
		status      string
		workspace   bool // platform_workspace is read_only for billing
		extraOrder  bool
		wantStatus  string
		wantRestore bool
	}{
		{"past_due", StatusPastDue, false, false, subscription.StatusActive, false},
		{"read_only", StatusReadOnly, true, false, subscription.StatusActive, true},
		{"another invoice still unpaid", StatusReadOnly, true, true, StatusReadOnly, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newUsageFixture(t, tc.status, &grace)
			if tc.workspace {
				if _, err := f.pool.Exec(context.Background(),
					`INSERT INTO platform_workspace (workspace_id, status, reason, changed_by) VALUES ($1, 'read_only', 'billing', $2)`, f.ws, systemUser); err != nil {
					t.Fatal(err)
				}
			}
			if tc.extraOrder {
				f.usageOrder(t0.Add(day))
			}
			cache := &cacheLog{}
			pay, err := billing.New(billing.Deps{
				Runner: billing.NewPgxRunner(f.pool), Provider: billing.ProviderManual,
				Workspaces: cache, Entitlements: policyCache{cache},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, version := f.subscription()
			applied, err := pay.ApplyPayment(context.Background(), billing.Confirmation{OrderID: f.orderID, ConfirmedBy: staffID})
			if err != nil {
				t.Fatal(err)
			}
			if applied.Order.Status != billing.StatusPaid {
				t.Fatalf("order = %+v, want paid", applied.Order)
			}
			status, graceUntil, after := f.state()
			if status != tc.wantStatus {
				t.Errorf("subscription status = %q, want %q", status, tc.wantStatus)
			}
			if tc.wantStatus == subscription.StatusActive && (graceUntil != nil || after != version+1) {
				t.Errorf("grace_until = %v, version %d; want it cleared and version %d", graceUntil, after, version+1)
			}
			var plan string
			if err := f.pool.QueryRow(context.Background(), `SELECT plan_key FROM workspace_subscription WHERE workspace_id = $1`, f.ws).Scan(&plan); err != nil || plan != f.planKey {
				t.Errorf("plan = %q (%v), want it unchanged: %q", plan, err, f.planKey)
			}
			ws, reason, found := f.workspaceState()
			switch {
			case tc.wantRestore && (!found || ws != string(workspacestate.Active)):
				t.Errorf("workspace = %q, %q, want active again", ws, reason)
			case tc.workspace && !tc.wantRestore && ws != string(workspacestate.ReadOnly):
				t.Errorf("workspace = %q, want it still read_only", ws)
			}
			if applied.WorkspaceReactivated != tc.wantRestore {
				t.Errorf("WorkspaceReactivated = %v, want %v", applied.WorkspaceReactivated, tc.wantRestore)
			}
		})
	}
}

func TestUsageLifecyclePeriodSkipsUsagePlans(t *testing.T) {
	f := newUsageFixture(t, subscription.StatusActive, nil)
	// The usage invoice is paid, so no debt edge applies; the month is over.
	if _, err := f.pool.Exec(context.Background(), `UPDATE billing_order SET status = 'paid' WHERE id = $1`, f.orderID); err != nil {
		t.Fatal(err)
	}
	f.run(t0.AddDate(0, 1, 0).Add(day))
	if status, _, version := f.state(); status != subscription.StatusActive || version != 3 {
		t.Fatalf("a usage subscription past its period = %q version %d, want active and unmoved", status, version)
	}
	if n := f.count(`SELECT count(*) FROM billing_notice WHERE workspace_id = $1`); n != 0 {
		t.Errorf("notices = %d, want none: no reminder, no state email", n)
	}

	// The reminder window and the cancel flag do not apply to it either.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE workspace_subscription SET current_period_end = $2, cancel_at_period_end = true WHERE workspace_id = $1`, f.ws, t0.AddDate(0, 1, 0).Add(2*day)); err != nil {
		t.Fatal(err)
	}
	f.run(t0.AddDate(0, 1, 0).Add(3 * day))
	if status, _, _ := f.state(); status != subscription.StatusActive {
		t.Errorf("status with cancel_at_period_end = %q, want active", status)
	}
}

// A usage invoice has no expires_at and never expires by itself (spec 13, AC34).
func TestOrderExpiryNeverTouchesAUsageOrder(t *testing.T) {
	f := newUsageFixture(t, subscription.StatusActive, nil)
	var expires *time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT expires_at FROM billing_order WHERE id = $1`, f.orderID).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if expires != nil {
		t.Fatalf("fixture usage order has expires_at %v, want none", expires)
	}
	x, err := NewOrderExpirer(NewPgxStore(f.pool), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Run(context.Background(), t0.AddDate(10, 0, 0)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.orderStatus(f.orderID); got != string(billing.StatusPending) {
		t.Fatalf("usage order status = %q, want pending", got)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'billing.order_expired'`); n != 0 {
		t.Fatalf("order_expired audit rows = %d, want 0", n)
	}
}
