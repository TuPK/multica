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
