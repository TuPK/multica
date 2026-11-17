package accountrequest

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// grantTx is the part of FinalizeTx that writeGrant uses. Any other method
// panics on the nil embedded interface.
type grantTx struct {
	FinalizeTx
	orders []billing.NewOrder
}

func (g *grantTx) InsertOrder(_ context.Context, n billing.NewOrder) (billing.Order, bool, error) {
	g.orders = append(g.orders, n)
	return billing.Order{ID: "order-1", Currency: n.Currency, Kind: n.Kind, PlanKey: n.PlanKey}, true, nil
}

func (g *grantTx) LockSubscription(context.Context, string) (subscription.Subscription, bool, error) {
	return subscription.Subscription{}, false, nil
}

func (g *grantTx) InsertSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, bool, error) {
	s.Version = 1
	return s, true, nil
}

func (g *grantTx) MarkOrderPaid(_ context.Context, id, _ string, _ time.Time) (billing.Order, error) {
	return billing.Order{ID: id, Currency: g.orders[0].Currency}, nil
}

func (g *grantTx) InsertAuditEvent(context.Context, audit.Entry) error { return nil }

func TestAccountRequestOrderUsesPlanCurrency(t *testing.T) {
	cases := map[string]struct{ planCurrency, want string }{
		"usd":        {"usd", "usd"},
		"upper case": {"USD", "usd"},
		"vnd":        {"vnd", "vnd"},
	}
	for name, tc := range cases {
		tx := &grantTx{}
		r := Request{ID: "req-1", WorkspaceID: "ws-1", WorkspaceName: "Acme", CreatedBy: adminA}
		p := plan.Plan{Key: "pro", Currency: tc.planCurrency}
		next := subscription.Subscription{PlanKey: "pro", Interval: subscription.IntervalMonth, Seats: 3, Status: subscription.StatusActive}
		staff := audit.Entry{ActorType: "user", ActorID: adminA}
		if err := (&Finalizer{}).writeGrant(context.Background(), tx, r, p, next, staff, t0); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(tx.orders) != 1 {
			t.Fatalf("%s: orders = %d, want 1", name, len(tx.orders))
		}
		if got := tx.orders[0].Currency; got != tc.want {
			t.Errorf("%s: order currency = %q, want %q", name, got, tc.want)
		}
	}
}
