package entitlement

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

func planWith(t *testing.T, key string, limit *int64) plan.Plan {
	t.Helper()
	p := catalogue(t)["free"]
	p.Key = key
	p.Limits = plan.Limits{plan.GateMaxWorkspaces: limit}
	return p
}

func TestTenantMaxWorkspacesIsLargestEffectiveLimit(t *testing.T) {
	free, pro, ent := catalogue(t)["free"], catalogue(t)["pro"], catalogue(t)["enterprise"]
	tests := []struct {
		name     string
		siblings []Sibling
		want     *int64
	}{
		{"one workspace", []Sibling{{Plan: pro}}, i64(3)},
		{"largest of the finite values", []Sibling{{Plan: free}, {Plan: pro}, {Plan: free}}, i64(3)},
		{"one empty value lifts the limit", []Sibling{{Plan: free}, {Plan: ent}, {Plan: pro}}, nil},
		{"an override raises the limit", []Sibling{{Plan: free, Overrides: plan.Limits{plan.GateMaxWorkspaces: i64(8)}}, {Plan: pro}}, i64(8)},
		{"an override can lower a workspace", []Sibling{{Plan: pro, Overrides: plan.Limits{plan.GateMaxWorkspaces: i64(2)}}, {Plan: free}}, i64(2)},
		{"an explicit null override lifts the limit", []Sibling{{Plan: free, Overrides: plan.Limits{plan.GateMaxWorkspaces: nil}}, {Plan: pro}}, nil},
		{"a plan with no key is no limit", []Sibling{{Plan: planWith(t, "x", nil)}, {Plan: free}}, nil},
	}
	for _, tc := range tests {
		got := TenantMaxWorkspaces(tc.siblings)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, deref(got), deref(tc.want))
		}
	}
}

func deref(p *int64) any {
	if p == nil {
		return "no limit"
	}
	return *p
}

func TestBuildMaxWorkspacesGate(t *testing.T) {
	free := catalogue(t)["free"]
	g := gateOf(t, Build(Inputs{Plan: free, WorkspaceStatus: "active", HasTenant: true, MaxWorkspaces: i64(3)}, now), plan.GateMaxWorkspaces)
	if g.Action != ActionEnforce || g.Limit == nil || *g.Limit != 3 || g.PeriodStart != nil {
		t.Errorf("tenant with limit 3: %+v, want enforce 3 with no period", g)
	}
	g = gateOf(t, Build(Inputs{Plan: free, WorkspaceStatus: "active", HasTenant: true}, now), plan.GateMaxWorkspaces)
	if g.Action != ActionOff || g.Limit != nil {
		t.Errorf("tenant with no limit: %+v, want off", g)
	}
	// A workspace with no tenant link (not backfilled yet) gets the gate off,
	// whatever its own plan says.
	g = gateOf(t, Build(Inputs{Plan: free, WorkspaceStatus: "active", MaxWorkspaces: i64(1)}, now), plan.GateMaxWorkspaces)
	if g.Action != ActionOff || g.Limit != nil {
		t.Errorf("no tenant link: %+v, want off", g)
	}
}

// --- Loader

type fakeTenants map[string][]string

func (f fakeTenants) SiblingIDs(_ context.Context, ws string) ([]string, error) { return f[ws], nil }

func tenantLoader(t *testing.T) Loader {
	ws := []string{wsA, wsB}
	return Loader{
		Plans: fakePlans{catalogue(t)},
		Subscriptions: fakeSubs{rows: map[string]subscription.Subscription{
			wsA: {WorkspaceID: wsA, PlanKey: "pro", Version: 2},
			// wsB has no row: it is on the free plan.
		}},
		Status:  fakeStatus{},
		Tenants: fakeTenants{wsA: ws, wsB: ws},
	}
}

func TestLoaderGivesEverySiblingTheTenantMaximum(t *testing.T) {
	l := tenantLoader(t)
	for _, id := range []string{wsA, wsB} {
		in, err := l.Inputs(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !in.HasTenant || in.MaxWorkspaces == nil || *in.MaxWorkspaces != 3 {
			t.Errorf("%s: HasTenant=%v MaxWorkspaces=%v, want the tenant's largest value 3", id, in.HasTenant, deref(in.MaxWorkspaces))
		}
	}
	// A workspace's own plan and version stay its own.
	inB, _ := l.Inputs(context.Background(), wsB)
	if inB.Plan.Key != "free" || inB.Version != 0 {
		t.Errorf("sibling inputs leaked into the workspace's own: %+v", inB)
	}
}

func TestLoaderWithoutATenantLinkHasNoTenant(t *testing.T) {
	l := tenantLoader(t)
	l.Tenants = fakeTenants{}
	in, err := l.Inputs(context.Background(), wsA)
	if err != nil {
		t.Fatal(err)
	}
	if in.HasTenant || in.MaxWorkspaces != nil {
		t.Errorf("inputs = %+v, want no tenant", in)
	}
	l.Tenants = nil
	if in, err := l.Inputs(context.Background(), wsA); err != nil || in.HasTenant {
		t.Errorf("a loader with no tenant reader: %+v, %v", in, err)
	}
}

func TestLoaderFailsWhenASiblingsPlanIsMissing(t *testing.T) {
	l := tenantLoader(t)
	l.Subscriptions = fakeSubs{rows: map[string]subscription.Subscription{wsB: {WorkspaceID: wsB, PlanKey: "gone"}}}
	if _, err := l.Inputs(context.Background(), wsA); err == nil {
		t.Fatal("no error when a sibling's plan is missing; the limit would be a guess")
	}
}
