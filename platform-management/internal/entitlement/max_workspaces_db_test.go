package entitlement

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// tenantWorkspace creates a workspace row and links it to tenant, in the tenant
// tables of spec R6 section 5.2. The rows go away with the test.
func (f *dbFixture) tenantWorkspace(tenant string, planKey string, overrides string) string {
	f.t.Helper()
	ws := f.newID()
	f.exec(`INSERT INTO workspace (id, name, slug) VALUES ($1, 'Tenant gate DB test', $2)`, ws, "tg-"+ws[:8])
	f.exec(`INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, ws, tenant)
	if planKey != "" {
		f.exec(`INSERT INTO workspace_subscription (workspace_id, plan_key, status, version, limit_overrides)
		        VALUES ($1, $2, 'active', 3, $3::jsonb)`, ws, planKey, overrides)
	}
	f.t.Cleanup(func() {
		ctx := context.Background()
		f.pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
	})
	return ws
}

func (f *dbFixture) newTenant() string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO tenant (id, name) VALUES ($1, 'Tenant gate DB test')`, id)
	f.t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM tenant WHERE id = $1`, id) })
	return id
}

func (f *dbFixture) planWithLimit(maxWorkspaces string) string {
	f.t.Helper()
	key := randomKey(f.t)
	f.keys = append(f.keys, key)
	f.exec(`INSERT INTO plan (key, name, currency, limits) VALUES ($1, 'DB tenant gate', 'vnd', $2::jsonb)`, key,
		`{"issue_count": null, "autopilot_runs": null, "max_workspaces": `+maxWorkspaces+`}`)
	return key
}

func TestPolicyCarriesTenantGateForEverySiblingWorkspace(t *testing.T) {
	f := newDBFixture(t)
	f.freePlan()
	small, big := f.planWithLimit("2"), f.planWithLimit("6")
	tenant := f.newTenant()
	a := f.tenantWorkspace(tenant, small, `{}`)
	b := f.tenantWorkspace(tenant, big, `{}`)
	alone := f.newID() // a workspace with no tenant link

	svc := f.service()
	for _, ws := range []string{a, b} {
		g := f.fetch(svc, ws).Gates[plan.GateMaxWorkspaces]
		if g.Action != ActionEnforce || limitOf(g) != 6 {
			t.Errorf("%s: max_workspaces = %+v, want enforce 6 (the larger of 2 and 6)", ws, g)
		}
	}
	if g := f.fetch(svc, alone).Gates[plan.GateMaxWorkspaces]; g.Action != ActionOff {
		t.Errorf("a workspace with no tenant link has the gate %+v, want off", g)
	}

	// Downgrading the larger workspace lowers the limit of both, once the cache is clear.
	f.exec(`UPDATE workspace_subscription SET plan_key = $2 WHERE workspace_id = $1`, b, small)
	svc.InvalidateAll()
	for _, ws := range []string{a, b} {
		if g := f.fetch(svc, ws).Gates[plan.GateMaxWorkspaces]; limitOf(g) != 2 {
			t.Errorf("%s after the plan change: max_workspaces = %+v, want 2", ws, g)
		}
	}

	// An override on one workspace reaches the other.
	f.exec(`UPDATE workspace_subscription SET limit_overrides = '{"max_workspaces": null}' WHERE workspace_id = $1`, a)
	svc.InvalidateAll()
	for _, ws := range []string{a, b} {
		if g := f.fetch(svc, ws).Gates[plan.GateMaxWorkspaces]; g.Action != ActionOff {
			t.Errorf("%s with an empty override on a sibling: %+v, want off", ws, g)
		}
	}
}
