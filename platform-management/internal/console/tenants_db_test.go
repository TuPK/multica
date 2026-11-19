package console

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Database tests of the tenant routes. Each test makes its own tenants and
// workspaces; the shared database may hold others, so lists are asserted on the
// rows of the test's own tenants.

func (f *dbFixture) namedTenant(name string) string {
	f.t.Helper()
	id := f.newTenantID()
	f.exec(`UPDATE tenant SET name = $2 WHERE id = $1`, id, name)
	return id
}

func (f *dbFixture) plan(maxWorkspaces string, sort int) string {
	f.t.Helper()
	key := "t" + randomHex(f.t)[:10]
	f.exec(`INSERT INTO plan (key, name, currency, sort, limits) VALUES ($1, 'Console tenant test', 'vnd', $2, $3::jsonb)`,
		key, sort, `{"max_workspaces": `+maxWorkspaces+`}`)
	f.t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM plan WHERE key = $1`, key) })
	return key
}

func (f *dbFixture) subscribe(ws, planKey, overrides string) {
	f.t.Helper()
	f.exec(`INSERT INTO workspace_subscription (workspace_id, plan_key, status, version, limit_overrides) VALUES ($1, $2, 'active', 1, $3::jsonb)`,
		ws, planKey, overrides)
	f.t.Cleanup(func() {
		f.pool.Exec(context.Background(), `DELETE FROM workspace_subscription WHERE workspace_id = $1`, ws)
	})
}

func (f *dbFixture) getJSON(user, path string) map[string]any {
	f.t.Helper()
	rec := f.do(user, "GET", path, "")
	if rec.Code != 200 {
		f.t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

func tenantIDsOf(list map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	rows, _ := list["tenants"].([]any)
	for _, r := range rows {
		row := r.(map[string]any)
		out[row["id"].(string)] = row
	}
	return out
}

func TestListTenantsSearchesTenantNameWorkspaceSlugAndOwnerEmail(t *testing.T) {
	f := newDBFixture(t)
	supportID, _ := f.staffUser(staff.RoleSupport)
	token := "zq" + randomHex(t)[:10]
	tenant := f.namedTenant("Tenant " + token)
	owner, ownerEmail := f.user("owner")
	member, _ := f.user("member")
	ws1 := f.workspaceIn(tenant, owner, member)
	ws2 := f.workspaceIn(tenant, owner, member)
	other := f.namedTenant("Other tenant")
	owner2, _ := f.user("owner")
	f.workspaceIn(other, owner2, member)

	var slug string
	if err := f.pool.QueryRow(context.Background(), `SELECT slug FROM workspace WHERE id = $1`, ws2).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	for name, q := range map[string]string{"tenant name": token, "workspace slug": slug, "owner email": ownerEmail} {
		got := tenantIDsOf(f.getJSON(supportID, "/api/platform/tenants?limit=100&q="+q))
		if _, ok := got[tenant]; !ok || got[other] != nil {
			t.Errorf("search by %s found %v, want the tenant %s and not %s", name, keys(got), tenant, other)
		}
	}
	row := tenantIDsOf(f.getJSON(supportID, "/api/platform/tenants?limit=100&q="+token))[tenant]
	if row["workspace_count"] != float64(2) || row["name"] != "Tenant "+token {
		t.Errorf("row = %v, want 2 workspaces", row)
	}
	_ = ws1
	if got := tenantIDsOf(f.getJSON(supportID, "/api/platform/tenants?limit=100&q=nomatch"+token)); len(got) != 0 {
		t.Errorf("a search with no match returned %v", keys(got))
	}
}

func keys(m map[string]map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestGetTenantListsWorkspacesWithEffectiveMaxWorkspaces(t *testing.T) {
	f := newDBFixture(t)
	supportID, _ := f.staffUser(staff.RoleSupport)
	tenant := f.namedTenant("Max workspaces " + randomHex(t)[:8])
	owner, ownerEmail := f.user("owner")
	member, _ := f.user("member")
	small, big := f.plan("2", 10), f.plan("6", 500)
	a := f.workspaceIn(tenant, owner, member)
	b := f.workspaceIn(tenant, owner, member)
	f.subscribe(a, small, `{}`)
	f.subscribe(b, big, `{}`)

	got := f.getJSON(supportID, "/api/platform/tenants/"+tenant)
	if got["max_workspaces"] != float64(6) || got["workspace_count"] != float64(2) || got["best_plan_key"] != big {
		t.Fatalf("tenant = %v, want max 6, 2 workspaces and the plan %s", got, big)
	}
	ws, _ := got["workspaces"].([]any)
	if len(ws) != 2 {
		t.Fatalf("workspaces = %v", got["workspaces"])
	}
	first := ws[0].(map[string]any)
	owned, _ := first["owner"].(map[string]any)
	if first["id"] != a || first["plan_key"] != small || first["member_count"] != float64(2) || owned["email"] != ownerEmail {
		t.Errorf("first workspace = %v", first)
	}

	// An override on one workspace raises the whole tenant's limit.
	f.exec(`UPDATE workspace_subscription SET limit_overrides = '{"max_workspaces": 9}' WHERE workspace_id = $1`, a)
	if got := f.getJSON(supportID, "/api/platform/tenants/"+tenant); got["max_workspaces"] != float64(9) {
		t.Errorf("max_workspaces = %v after an override of 9, want 9", got["max_workspaces"])
	}
	// An explicit empty override lifts it: null, not 0.
	f.exec(`UPDATE workspace_subscription SET limit_overrides = '{"max_workspaces": null}' WHERE workspace_id = $1`, a)
	if got := f.getJSON(supportID, "/api/platform/tenants/"+tenant); got["max_workspaces"] != nil {
		t.Errorf("max_workspaces = %v after an empty override, want null", got["max_workspaces"])
	}
	// A workspace with no subscription is on the free plan.
	c := f.workspaceIn(tenant, owner, member)
	var found bool
	for _, w := range f.getJSON(supportID, "/api/platform/tenants/"+tenant)["workspaces"].([]any) {
		if w.(map[string]any)["id"] == c {
			found = w.(map[string]any)["plan_key"] == "free"
		}
	}
	if !found {
		t.Errorf("the workspace with no subscription is not shown on the free plan")
	}
}

func TestTenantStatusSummaryCountsWorkspaceStatuses(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	tenant := f.namedTenant("Status summary " + randomHex(t)[:8])
	owner, _ := f.user("owner")
	member, _ := f.user("member")
	a := f.workspaceIn(tenant, owner, member)
	b := f.workspaceIn(tenant, owner, member)
	f.workspaceIn(tenant, owner, member)
	for ws, status := range map[string]string{a: "suspended", b: "read_only"} {
		if rec := f.do(adminID, "POST", "/api/platform/workspaces/"+ws+"/status", `{"status":"`+status+`","reason":"summary test"}`); rec.Code != 200 {
			t.Fatalf("set %s: %d %s", status, rec.Code, rec.Body.String())
		}
	}
	got := f.getJSON(adminID, "/api/platform/tenants/"+tenant)
	summary, _ := got["status_summary"].(map[string]any)
	if summary["suspended"] != float64(1) || summary["read_only"] != float64(1) || summary["active"] != float64(1) || len(summary) != 3 {
		t.Errorf("status_summary = %v, want one workspace in each of active, suspended and read_only", summary)
	}
	row := tenantIDsOf(f.getJSON(adminID, "/api/platform/tenants?limit=100&q=Status%20summary"))[tenant]
	if s, _ := row["status_summary"].(map[string]any); s["suspended"] != float64(1) {
		t.Errorf("the list row's status_summary = %v", row["status_summary"])
	}
}

// Locking a workspace is that workspace's row alone: the tenant has no state of
// its own to move, and the siblings stay as they were.
func TestLockingOneWorkspaceLeavesSiblingsUntouched(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	tenant := f.namedTenant("Siblings " + randomHex(t)[:8])
	owner, _ := f.user("owner")
	member, _ := f.user("member")
	a := f.workspaceIn(tenant, owner, member)
	b := f.workspaceIn(tenant, owner, member)

	if rec := f.do(adminID, "POST", "/api/platform/workspaces/"+a+"/status", `{"status":"suspended","reason":"lock one"}`); rec.Code != 200 {
		t.Fatalf("lock = %d %s", rec.Code, rec.Body.String())
	}
	if n := f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1 AND status = 'suspended'`, a); n != 1 {
		t.Errorf("the locked workspace has %d suspended rows, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, b); n != 0 {
		t.Errorf("the sibling got %d platform rows, want none", n)
	}
	if got := f.getJSON(adminID, "/api/platform/workspaces/"+b); got["status"] != "active" {
		t.Errorf("the sibling's status = %v, want active", got["status"])
	}
	if n := f.count(`SELECT count(*) FROM tenant WHERE id = $1 AND status = 'active'`, tenant); n != 1 {
		t.Errorf("the tenant's own status changed")
	}
}

func TestOverviewCountsWorkspacesWithoutTenant(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	owner, _ := f.user("owner")
	member, _ := f.user("member")
	count := func() float64 {
		t.Helper()
		v, _ := f.getJSON(adminID, "/api/platform/overview")["workspaces_without_tenant"].(float64)
		return v
	}
	before := count()
	f.workspaceIn("", owner, member) // no link: a workspace an old server created
	if got := count(); got != before+1 {
		t.Errorf("workspaces_without_tenant = %v, want %v after an orphan workspace", got, before+1)
	}
	f.workspace(owner, member) // with its tenant
	if got := count(); got != before+1 {
		t.Errorf("workspaces_without_tenant = %v, want %v: a workspace with a tenant is not counted", got, before+1)
	}
}
