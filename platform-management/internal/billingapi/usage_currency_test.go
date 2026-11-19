package billingapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

type fakeActivator struct{ got []billing.UsageActivation }

func (f *fakeActivator) ActivateUsage(_ context.Context, a billing.UsageActivation) (billing.Order, bool, error) {
	f.got = append(f.got, a)
	return billing.Order{ID: "dddddddd-0000-0000-0000-000000000002"}, true, nil
}

// The activation order of a usage plan is in the plan's currency (spec 7.4, rule 1).
func TestUsageActivationCarriesPlanCurrency(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountIndividual)
	e.subs.profiles[wsID] = subscription.BillingProfile{
		WorkspaceID: wsID, AccountKind: subscription.AccountIndividual, LegalName: "An", InvoiceEmail: "an@example.test",
	}
	for _, p := range e.plans.plans {
		if p.BillingModel == plan.BillingUsage {
			p.Key, p.Currency = "usdusage", "usd"
			e.plans.plans = append(e.plans.plans, p)
			break
		}
	}
	act := &fakeActivator{}
	h, err := New(Deps{
		Subscriptions: e.subs, Plans: e.plans, Members: e.members, Orders: e.orders, Workspaces: e.spaces, Activator: act,
		IndividualMaxSeats: 5, VATRate: billing.DefaultVATRate, AppURL: appURL,
		Now: func() time.Time { return now },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.h = h.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "plan_key": "usdusage", "interval": "month", "idempotency_key": "usd-usage"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(act.got) != 1 || act.got[0].Currency != "usd" {
		t.Fatalf("activations = %+v, want one in usd", act.got)
	}
}
