package billingapi

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// addUSDPlan adds a copy of the Pro plan priced in usd under the key "usdplan".
func (e *env) addUSDPlan(t *testing.T) {
	t.Helper()
	for _, p := range e.plans.plans {
		if p.Key == "pro" {
			p.Key = "usdplan"
			p.Currency = "usd"
			e.plans.plans = append(e.plans.plans, p)
			return
		}
	}
	t.Fatal("no pro plan in the catalogue")
}

func TestOrderCurrencyEqualsPlanCurrency(t *testing.T) {
	e := newEnv(t)
	e.addUSDPlan(t)
	rec := e.checkout(ownerID, obj{
		"workspace_id": wsID, "plan_key": "usdplan", "interval": "month", "idempotency_key": "usd-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(e.orders.drafts) != 1 {
		t.Fatalf("drafts = %d, want 1", len(e.orders.drafts))
	}
	if got := e.orders.drafts[0].Currency; got != "usd" {
		t.Errorf("draft currency = %q, want usd", got)
	}
}

func TestAddSeatsUsesSubscriptionCurrency(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	e.addUSDPlan(t)
	s := e.paid()
	s.PlanKey = "usdplan"
	e.subs.rows[wsID] = s

	q := e.quote(t)
	if q["currency"] != "usd" {
		t.Fatalf("quote currency = %v, want usd", q["currency"])
	}

	wrong := confirm(q, "usd-wrong")
	wrong["currency"] = "vnd"
	wantCode(t, e.do("POST", purchasePath(wsID), ownerID, nil, wrong), http.StatusBadRequest, "invalid_request")
	if len(e.orders.drafts) != 0 {
		t.Fatal("a purchase in the wrong currency wrote an order")
	}

	rec := e.do("POST", purchasePath(wsID), ownerID, nil, confirm(q, "usd-ok"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(e.orders.drafts) != 1 || e.orders.drafts[0].Currency != "usd" {
		t.Fatalf("drafts = %+v, want one in usd", e.orders.drafts)
	}
	if m := decode(t, rec); m["currency"] != "usd" {
		t.Errorf("response currency = %v, want usd", m["currency"])
	}
}
