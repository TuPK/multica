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
