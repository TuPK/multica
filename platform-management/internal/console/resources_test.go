package console

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

const secondWsID = "bbbbbbbb-0000-0000-0000-000000000002"

// setResourceFixture gives the tenant two workspaces: the first is at 90 percent
// of its agent limit and over its issue limit, the second is at half its agent
// limit. The tenant allows three workspaces and has two.
func setResourceFixture(e *env) {
	e.reader.details[wsID] = WorkspaceDetail{Current: usage.Current{Agents: 9, Runtimes: 3, IssuesTotal: 12}, GroupCount: 1}
	e.reader.details[secondWsID] = WorkspaceDetail{Current: usage.Current{Agents: 1}}
	d := e.reader.tenantDetails[tenantID]
	d.MaxWorkspaces = i64(3)
	d.Workspaces = []TenantWorkspace{
		{ID: wsID, Slug: "acme", PlanKey: "pro"},
		{ID: secondWsID, Slug: "second", PlanKey: "free"},
	}
	e.reader.tenantDetails[tenantID] = d
	e.reader.limits = map[string]map[string]*int64{
		wsID: {
			plan.GateAgents: i64(10), plan.GateRuntimes: nil, plan.GateIssueCount: i64(10),
			plan.GateStorageBytes: i64(100), plan.GateMemberGroups: i64(5),
		},
		secondWsID: {
			plan.GateAgents: i64(2), plan.GateRuntimes: nil, plan.GateIssueCount: nil,
			plan.GateStorageBytes: nil, plan.GateMemberGroups: nil,
		},
	}
}

// resourceOf returns the entry of one type in a resource list.
func resourceOf(t *testing.T, list []any, typ string) map[string]any {
	t.Helper()
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["type"] == typ {
			return m
		}
	}
	t.Fatalf("no resource %s in %v", typ, list)
	return nil
}

// TestTenantResourcesAddUpAndStateEachLimit: the tenant totals add its
// workspaces, carry no limit of their own, and max_workspaces is the one tenant
// limit. Each workspace states its used count against its own limit.
func TestTenantResourcesAddUpAndStateEachLimit(t *testing.T) {
	e := newEnv(t)
	setResourceFixture(e)

	rec := e.do(suppID, "GET", "/api/platform/tenants/"+tenantID+"/resources", "")
	wantStatus(t, rec, http.StatusOK)
	body := decode(t, rec)

	total, _ := body["resources"].([]any)
	if got := resourceOf(t, total, plan.GateAgents); got["used"] != float64(10) || got["limit"] != nil || got["status"] != "ok" {
		t.Fatalf("tenant total = %v", got)
	}
	if got := resourceOf(t, total, plan.GateMaxWorkspaces); got["used"] != float64(2) || got["limit"] != float64(3) || got["status"] != "ok" {
		t.Fatalf("max_workspaces = %v", got)
	}

	workspaces, _ := body["workspaces"].([]any)
	if len(workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(workspaces))
	}
	first, _ := workspaces[0].(map[string]any)
	firstRes, _ := first["resources"].([]any)
	if agents := resourceOf(t, firstRes, plan.GateAgents); agents["percent"] != float64(90) || agents["status"] != "warning" {
		t.Fatalf("acme %s = %v, want 90 percent warning", plan.GateAgents, agents)
	}
	if issues := resourceOf(t, firstRes, plan.GateIssueCount); issues["status"] != "over" {
		t.Fatalf("acme %s = %v, want over", plan.GateIssueCount, issues)
	}
	if runtimes := resourceOf(t, firstRes, plan.GateRuntimes); runtimes["limit"] != nil || runtimes["status"] != "ok" {
		t.Fatalf("acme %s = %v, want no limit", plan.GateRuntimes, runtimes)
	}
}

// TestTenantResourcesScopeAnswersOutsideTenantAsNotFound: an admin sees its own
// tenant only; a tenant outside its scope answers as a missing one.
func TestTenantResourcesScopeAnswersOutsideTenantAsNotFound(t *testing.T) {
	e := newEnv(t)
	setResourceFixture(e)
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "GET", "/api/platform/tenants/"+otherTenantID+"/resources", ""))
}

// TestSystemResourcesTotalsTopAndOverLimit: system_admin sees the page totals,
// the top tenants of each type, and the workspaces that are warning or over.
func TestSystemResourcesTotalsTopAndOverLimit(t *testing.T) {
	e := newEnv(t)
	setResourceFixture(e)
	e.reader.tenantRows = []TenantRow{{ID: tenantID, CreatedAt: t0}}

	rec := e.do(adminID, "GET", "/api/platform/resources?top=1", "")
	wantStatus(t, rec, http.StatusOK)
	body := decode(t, rec)

	if body["complete"] != true || body["next_cursor"] != nil {
		t.Fatalf("complete = %v, next_cursor = %v; a one-page result is complete", body["complete"], body["next_cursor"])
	}
	totals, _ := body["totals"].([]any)
	if got := resourceOf(t, totals, plan.GateAgents); got["used"] != float64(10) {
		t.Fatalf("totals %s = %v", plan.GateAgents, got)
	}
	top, _ := body["top"].(map[string]any)
	agents, _ := top[plan.GateAgents].([]any)
	if len(agents) != 1 {
		t.Fatalf("top %s = %v, want one tenant", plan.GateAgents, agents)
	}
	if first, _ := agents[0].(map[string]any); first["tenant_id"] != tenantID {
		t.Fatalf("top %s = %v", plan.GateAgents, agents)
	}
	over, _ := body["over_limit"].([]any)
	found := false
	for _, item := range over {
		m, _ := item.(map[string]any)
		if m["workspace_id"] == wsID && m["type"] == plan.GateIssueCount && m["status"] == "over" {
			found = true
		}
	}
	if !found {
		t.Fatalf("over_limit = %v, want the acme %s", over, plan.GateIssueCount)
	}
}

// TestSystemResourcesIsSystemAdminOnly: an admin, even of the tenant, gets a
// flat 404 on the system page.
func TestSystemResourcesIsSystemAdminOnly(t *testing.T) {
	e := newEnv(t)
	setResourceFixture(e)
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "GET", "/api/platform/resources", ""))
}
