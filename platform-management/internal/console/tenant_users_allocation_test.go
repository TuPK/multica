// Spec R6 section 3b.3 rule 2 and D95: the user detail shows the user's seats and
// each workspace's effective limits and usage; admin has no route to allocate.

package console

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

func setAllocationFixture(e *env) {
	d := e.reader.tenantDetails[tenantID]
	d.Workspaces = []TenantWorkspace{{ID: tuWS, Slug: "acme", PlanKey: "pro"}}
	e.reader.tenantDetails[tenantID] = d
	e.reader.details[tuWS] = WorkspaceDetail{Current: usage.Current{Agents: 9}}
	e.reader.limits = map[string]map[string]*int64{tuWS: {plan.GateAgents: i64(10)}}
}

func TestUserDetailAllocationShowsSeatsAndLimits(t *testing.T) {
	e := tenantUserEnv(t)
	setAllocationFixture(e)
	rec := e.do(tadminID, "GET", "/api/platform/tenants/"+tenantID+"/users/"+tuMem, "")
	wantStatus(t, rec, http.StatusOK)
	alloc, _ := decode(t, rec)["allocation"].(map[string]any)
	ws, _ := alloc["workspaces"].([]any)
	if len(ws) != 1 {
		t.Fatalf("allocation = %v", alloc)
	}
	first, _ := ws[0].(map[string]any)
	if first["workspace_id"] != tuWS || first["member_role"] != "member" || first["occupies_seat"] != true {
		t.Fatalf("entry = %v", first)
	}
	res, _ := first["resources"].([]any)
	gate := resourceOf(t, res, plan.GateAgents)
	if gate["used"] != float64(9) || gate["limit"] != float64(10) || gate["status"] != "warning" {
		t.Fatalf("limited gate = %v", gate)
	}
}

func TestAllocationRoutesRefuseAdmin(t *testing.T) {
	e := tenantUserEnv(t)
	setAllocationFixture(e)
	for _, m := range []string{"PUT", "POST"} {
		wantUnknownRouteOrNotFound(t, e.do(tadminID, m, "/api/platform/tenants/"+tenantID+"/users/"+tuMem+"/allocation", `{"seats":5}`))
	}
}
