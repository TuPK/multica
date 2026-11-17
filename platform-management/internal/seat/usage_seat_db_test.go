package seat

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

type noWorkspaceCache struct{}

func (noWorkspaceCache) Invalidate(context.Context, string) error { return nil }

type noEntitlementCache struct{}

func (noEntitlementCache) Invalidate(string) {}
func (noEntitlementCache) InvalidateAll()    {}

// useUsagePlan moves the fixture's workspace onto a pay as you go plan: seats 0
// and no seat price (spec 7b.3, rule 5).
func (f *dbFixture) useUsagePlan() {
	f.t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE plan SET billing_model = 'usage', seat_price_month = NULL, seat_price_year = NULL WHERE key = $1`, f.plan); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE workspace_subscription SET seats = 0 WHERE workspace_id = $1`, f.ws); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dbFixture) memberCount() int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM member WHERE workspace_id = $1`, f.ws).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// AC32: a usage plan has no seats to manage, so an invitation and a share-link
// join are not refused for seats, however many members the workspace has.
func TestDBInvitationsAndShareLinksAreNotBlockedBySeatsOnAUsagePlan(t *testing.T) {
	f := newDBFixture(t, 0, 4)
	f.useUsagePlan()
	ctx := context.Background()

	id := newUUID(t)
	invite, err := f.svc.Reserve(ctx, f.ws, id, id, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if invite.Managed || invite.Reason != "" || invite.Capacity != nil || invite.Operation != nil {
		t.Fatalf("invitation decision = %+v, want unmanaged with no refusal", invite)
	}
	join := newUUID(t)
	share, err := f.svc.Claim(ctx, f.ws, join, join)
	if err != nil {
		t.Fatal(err)
	}
	if share.Managed || share.Reason != "" || share.Capacity != nil || share.Operation != nil {
		t.Fatalf("share-link decision = %+v, want unmanaged with no refusal", share)
	}
}

// AC23: a seat decrease that takes effect at renewal below the member count
// removes nobody, and the workspace is over capacity: it admits no new member
// until members leave or seats are bought.
func TestDBSeatDecreaseBelowTheMemberCountRemovesNobodyAndOvercommits(t *testing.T) {
	f := newDBFixture(t, 5, 5)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE plan SET seat_price_month = 100000, seat_price_year = 1000000 WHERE key = $1`, f.plan); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Second).Add(-30 * 24 * time.Hour)
	end := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	if _, err := f.pool.Exec(ctx, `UPDATE workspace_subscription
		SET interval = 'month', current_period_start = $2, current_period_end = $3,
		    pending_change = jsonb_build_object('plan_key', plan_key, 'seats', 3, 'interval', 'month')
		WHERE workspace_id = $1`, f.ws, start, end); err != nil {
		t.Fatal(err)
	}

	cur, found, err := subscription.NewPgxReader(f.pool).Get(ctx, f.ws)
	if err != nil || !found || cur.PendingChange == nil || cur.PendingChange.Seats != 3 {
		t.Fatalf("subscription = %+v found %v err %v, want a pending change to 3 seats", cur, found, err)
	}
	p, found, err := plan.NewPgxReader(f.pool).Get(ctx, f.plan)
	if err != nil || !found {
		t.Fatalf("plan: found %v err %v", found, err)
	}
	draft, err := billing.RenewalDraft(cur, p, end)
	if err != nil {
		t.Fatal(err)
	}
	orders, err := billing.New(billing.Deps{
		Runner: billing.NewPgxRunner(f.pool), Provider: billing.ProviderManual,
		Workspaces: noWorkspaceCache{}, Entitlements: noEntitlementCache{},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft.IdempotencyKey = "renew-" + suffix(t)
	o, _, err := orders.CreateOrder(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.pool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(context.Background(), `DELETE FROM billing_order WHERE workspace_id = $1`, f.ws)
	})
	if _, err := orders.ApplyPayment(ctx, billing.Confirmation{OrderID: o.ID, ConfirmedBy: newUUID(t)}); err != nil {
		t.Fatal(err)
	}

	after, _, err := subscription.NewPgxReader(f.pool).Get(ctx, f.ws)
	if err != nil {
		t.Fatal(err)
	}
	if after.Seats != 3 || after.PendingChange != nil {
		t.Fatalf("after the renewal: seats %d pending %+v, want 3 seats and no pending change", after.Seats, after.PendingChange)
	}
	if n := f.memberCount(); n != 5 {
		t.Fatalf("members after the seat decrease = %d, want all 5 kept", n)
	}
	id := newUUID(t)
	if _, err := f.svc.Reserve(ctx, f.ws, id, id, time.Now().Add(time.Hour)); err != ErrOvercommitted {
		t.Fatalf("Reserve error = %v, want ErrOvercommitted for 5 members on 3 seats", err)
	}
}
