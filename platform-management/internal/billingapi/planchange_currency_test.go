package billingapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func TestChangePlanAcrossCurrenciesRefused(t *testing.T) {
	e := newEnv(t)
	e.paid()
	var target plan.Plan
	for _, p := range e.plans.plans {
		if p.Key == "unlimited" {
			target = p
		}
	}
	target.Key = "usdplan"
	target.Currency = "usd"
	e.plans.plans = append(e.plans.plans, target)
	rec := e.do(http.MethodPost, planChangePath(wsID)+"/preview", ownerID, nil, obj{"plan_key": "usdplan", "interval": "month"})
	wantCode(t, rec, http.StatusConflict, "plan_change_unavailable")
	if !strings.Contains(rec.Body.String(), "currency_mismatch") {
		t.Fatalf("body %s does not give reason currency_mismatch", rec.Body.String())
	}
	if e.changeOrder.creates != 0 {
		t.Fatal("a refused change wrote an order")
	}
}

// A checkout is an order, so it follows the same currency gate as a plan change
// (spec 7.4, rule 1).
func TestCheckoutAcrossCurrenciesRefused(t *testing.T) {
	e := newEnv(t)
	e.paid()
	for i := range e.plans.plans {
		if e.plans.plans[i].Key == "pro" {
			e.plans.plans[i].Currency = "usd"
		}
	}
	rec := e.checkout(ownerID, obj{
		"workspace_id": wsID, "plan_key": "unlimited", "interval": "month", "idempotency_key": "cross-1",
	})
	wantCode(t, rec, http.StatusConflict, "plan_change_unavailable")
	if !strings.Contains(rec.Body.String(), "currency_mismatch") {
		t.Fatalf("body %s does not give reason currency_mismatch", rec.Body.String())
	}
	if len(e.orders.drafts) != 0 {
		t.Fatalf("a refused checkout wrote %d drafts", len(e.orders.drafts))
	}
}
