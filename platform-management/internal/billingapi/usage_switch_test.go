package billingapi

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// AC23 (spec 7b.1, rule 7): a workspace on a paid seat plan never moves to a
// usage plan at once. The change is either held for the end of the paid period
// or refused; it is not an order that takes effect on payment, and the
// subscription keeps its seat plan.
func TestSwitchFromASeatPlanToAUsagePlanIsNeverImmediate(t *testing.T) {
	e := newEnv(t)
	before := e.paid()
	e.plans.plans = append(e.plans.plans, plan.Plan{
		Key: "payg", Name: "Pay as you go", Currency: "vnd", IsPublic: true, Audience: plan.AudienceAny,
		BillingModel: plan.BillingUsage, Limits: plan.Limits{}, Features: map[string]bool{},
	})
	e.changeOrder.amount = 0

	rec := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil,
		obj{"plan_key": "payg", "interval": "month", "accepted_amount": 0, "idempotency_key": "usage-1"})
	if e.changeOrder.creates != 0 {
		t.Fatalf("a switch to a usage plan created %d change orders: %s", e.changeOrder.creates, rec.Body)
	}
	switch rec.Code {
	case http.StatusOK:
		got := decode(t, rec)
		if got["mode"] != "schedule" || got["effective_at"] != rfc3339Text(periodEnd) {
			t.Fatalf("response = %v, want a schedule effective at the period end %s", got, rfc3339Text(periodEnd))
		}
	case http.StatusBadRequest, http.StatusConflict:
	default:
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := e.subs.rows[wsID]; got.PlanKey != before.PlanKey || got.Seats != before.Seats || got.Status != before.Status {
		t.Fatalf("subscription = %+v, want the seat plan kept", got)
	}
}
