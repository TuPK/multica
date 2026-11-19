package plan

import (
	"slices"
	"testing"
)

// The defaults of spec R6 section 5.6. An empty value is an explicit null in
// the catalogue, so a reader can tell "no limit" from "forgot to set it".
func TestCatalogueMaxWorkspacesDefaults(t *testing.T) {
	plans, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]*int64{
		"free": i64(1), "pro": i64(3), "business": i64(10), "payg": i64(5),
		"enterprise": nil, "unlimited": nil,
	}
	if len(plans) != len(want) {
		t.Fatalf("catalogue has %d plans, want %d", len(plans), len(want))
	}
	for _, p := range plans {
		w, ok := want[p.Key]
		if !ok {
			t.Errorf("unexpected plan %q", p.Key)
			continue
		}
		got, present := p.Limits[GateMaxWorkspaces]
		if !present {
			t.Errorf("%s has no max_workspaces key; an empty value must be an explicit null", p.Key)
			continue
		}
		if (got == nil) != (w == nil) || (w != nil && *got != *w) {
			t.Errorf("%s max_workspaces = %v, want %v", p.Key, deref(got), deref(w))
		}
	}
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestValidateRejectsMaxWorkspacesBelowOne(t *testing.T) {
	base := func(limit *int64, set bool) Plan {
		p := Plan{Key: "team", Name: "Team", Currency: "vnd", Limits: Limits{}}
		if set {
			p.Limits[GateMaxWorkspaces] = limit
		}
		return p
	}
	for _, tc := range []struct {
		name  string
		limit *int64
		set   bool
		ok    bool
	}{
		{"zero", i64(0), true, false},
		{"negative", i64(-1), true, false},
		{"one", i64(1), true, true},
		{"many", i64(250), true, true},
		{"explicit null", nil, true, true},
		{"absent", nil, false, true},
	} {
		if err := Validate(base(tc.limit, tc.set)); (err == nil) != tc.ok {
			t.Errorf("%s: Validate = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
	if err := ValidateLimits(Limits{GateMaxWorkspaces: i64(0)}); err == nil {
		t.Error("a workspace override of 0 was accepted; the same rule applies to overrides")
	}
}

// max_workspaces is a counted limit, so the unlimited plan leaves it empty and a
// value on that plan is refused (D42).
func TestUnlimitedPlanStillLeavesEveryCountedGateEmpty(t *testing.T) {
	if !slices.Contains(CountedGates, GateMaxWorkspaces) {
		t.Fatal("max_workspaces is not in CountedGates")
	}
	if !slices.Contains(LimitGates, GateMaxWorkspaces) {
		t.Fatal("max_workspaces is not in LimitGates")
	}
	plans, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.Key != UnlimitedKey {
			continue
		}
		if !IsUnlimited(p) {
			t.Errorf("the catalogue's unlimited plan sets a counted limit: %v", p.Limits)
		}
		p.Limits = Limits{GateMaxWorkspaces: i64(2)}
		if err := Validate(p); err == nil {
			t.Error("the unlimited plan accepts a max_workspaces value")
		}
	}
}
