package billingapi

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Spec R6 section 3b.1 rule 5 and D87: a deactivated plan is refused as
// plan_inactive, not as not found, to a checkout that names it.
func TestDeactivatedPlanIsPlanInactive(t *testing.T) {
	at := time.Now()
	plans := []plan.Plan{
		{Key: "pro", IsPublic: true, ArchivedAt: &at},
		{Key: "team", IsPublic: true},
	}
	if !planInactive(plans, "pro", nil) {
		t.Fatal("archived plan named at checkout must be plan_inactive")
	}
	if planInactive(plans, "team", nil) {
		t.Fatal("active plan must not be plan_inactive")
	}
	if planInactive(plans, "missing", nil) {
		t.Fatal("unknown key is not plan_inactive")
	}
}

// Spec R6 section 3b.1 rule 5: a checkout that names a deactivated plan is refused
// as plan_inactive and creates no order. The hiding from the workspace's own plan
// list is served by Multica (server/), not by this module.
func TestInactivePlanIsHiddenFromWorkspacePlansAndRefusedAtCheckout(t *testing.T) {
	e := newEnv(t)
	at := time.Now()
	e.plans.plans = append(e.plans.plans, plan.Plan{Key: "retired", Name: "Retired", IsPublic: true, ArchivedAt: &at})

	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "retired"})
	wantCode(t, rec, 409, "plan_inactive")
	if len(e.orders.drafts) != 0 {
		t.Error("an order was created for an inactive plan")
	}
}
