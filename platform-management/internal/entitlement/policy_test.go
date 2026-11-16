package entitlement

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func i64(n int64) *int64 { return &n }

// at parses an RFC 3339 instant.
func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// catalogue returns the embedded plans by key, as the policy sees them.
func catalogue(t *testing.T) map[string]plan.Plan {
	t.Helper()
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]plan.Plan{}
	for _, p := range all {
		out[p.Key] = p
	}
	return out
}

func gateOf(t *testing.T, p Policy, name string) Gate {
	t.Helper()
	g, ok := p.Gates[name]
	if !ok {
		t.Fatalf("policy has no %s gate; gates = %v", name, p.Gates)
	}
	return g
}

var now = time.Date(2026, 8, 15, 10, 30, 45, 123, time.UTC)

// A workspace with no subscription row is on the free plan, at version 0.
func TestBuildFreeWorkspace(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "active"}, now)

	if p.SchemaVersion != 1 || p.PolicyRevision <= 0 || p.SubscriptionVersion != 0 {
		t.Fatalf("header = schema %d, revision %d, version %d", p.SchemaVersion, p.PolicyRevision, p.SubscriptionVersion)
	}
	if p.ValidForSeconds != 30 {
		t.Errorf("valid_for_seconds = %d, want 30", p.ValidForSeconds)
	}
	if !p.ValidUntil.Equal(time.Date(2026, 8, 15, 10, 31, 15, 0, time.UTC)) {
		t.Errorf("valid_until = %v, want now plus 30 seconds", p.ValidUntil)
	}
	issues := gateOf(t, p, "issue_count")
	if issues.Action != "enforce" || issues.Limit == nil || *issues.Limit != 200 {
		t.Errorf("issue_count = %+v, want enforce 200", issues)
	}
	runs := gateOf(t, p, "autopilot_runs")
	if runs.Action != "enforce" || runs.Limit == nil || *runs.Limit != 50 {
		t.Errorf("autopilot_runs = %+v, want enforce 50", runs)
	}
	if len(p.Gates) != len(wantGateNames) {
		t.Errorf("gates = %d, want %d: %v", len(p.Gates), len(wantGateNames), p.Gates)
	}
	for _, name := range wantGateNames {
		gateOf(t, p, name)
	}
}

// Overrides win over the plan, per gate.
func TestBuildPaidWorkspaceWithOverrides(t *testing.T) {
	p := Build(Inputs{
		Plan:            catalogue(t)["pro"], // issue_count unlimited, autopilot_runs 2000
		Version:         7,
		Overrides:       plan.Limits{"issue_count": i64(500), "autopilot_runs": i64(3000)},
		WorkspaceStatus: "active",
	}, now)

	if p.SubscriptionVersion != 7 {
		t.Errorf("subscription_version = %d, want the row's version 7", p.SubscriptionVersion)
	}
	if g := gateOf(t, p, "issue_count"); g.Action != "enforce" || *g.Limit != 500 {
		t.Errorf("issue_count = %+v, want the override 500 over the plan's unlimited", g)
	}
	if g := gateOf(t, p, "autopilot_runs"); g.Action != "enforce" || *g.Limit != 3000 {
		t.Errorf("autopilot_runs = %+v, want the override 3000 over the plan's 2000", g)
	}
}

func TestBuildUsesThePlanWhereThereIsNoOverride(t *testing.T) {
	p := Build(Inputs{
		Plan:            catalogue(t)["pro"],
		Overrides:       plan.Limits{"agents": i64(5)}, // a gate other than the two asserted below
		WorkspaceStatus: "active",
	}, now)
	if g := gateOf(t, p, "autopilot_runs"); *g.Limit != 2000 {
		t.Errorf("autopilot_runs = %+v, want the plan's 2000", g)
	}
	if g := gateOf(t, p, "issue_count"); g.Action != "off" {
		t.Errorf("issue_count = %+v, want the plan's unlimited", g)
	}
}

// An empty value is unlimited: it is off, and it lifts a plan's limit.
func TestBuildEmptyOverrideMeansUnlimited(t *testing.T) {
	p := Build(Inputs{
		Plan:            catalogue(t)["free"],
		Overrides:       plan.Limits{"autopilot_runs": nil, "issue_count": nil},
		WorkspaceStatus: "active",
	}, now)
	for _, name := range []string{"issue_count", "autopilot_runs"} {
		g := gateOf(t, p, name)
		if g.Action != "off" || g.Limit != nil {
			t.Errorf("%s = %+v, want off with no limit", name, g)
		}
	}
}

// enforce carries a non-negative limit; off carries none.
func TestBuildEnforceVersusOff(t *testing.T) {
	enforce := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "active"}, now)
	off := Build(Inputs{Plan: catalogue(t)["pro"], WorkspaceStatus: "active"}, now)

	if g := gateOf(t, enforce, "issue_count"); g.Action != "enforce" || g.Limit == nil || *g.Limit < 0 {
		t.Errorf("limited gate = %+v, want enforce with a non-negative limit", g)
	}
	g := gateOf(t, off, "issue_count")
	if g.Action != "off" || g.Limit != nil {
		t.Errorf("unlimited gate = %+v, want off with no limit", g)
	}
	raw, _ := json.Marshal(g)
	if string(raw) != `{"action":"off"}` {
		t.Errorf("off gate encodes as %s, want only the action", raw)
	}
}

func TestBuildNonActiveWorkspaceLimitsAutopilotRunsToZero(t *testing.T) {
	for _, status := range []string{"read_only", "suspended", "pending_deletion"} {
		for _, key := range []string{"free", "pro", "enterprise"} { // limited and unlimited alike
			in := Inputs{Plan: catalogue(t)[key], WorkspaceStatus: status}
			if key == "enterprise" {
				in.Overrides = plan.Limits{"autopilot_runs": nil} // an explicitly unlimited workspace
			}
			p := Build(in, now)

			runs := gateOf(t, p, "autopilot_runs")
			if runs.Action != "enforce" || runs.Limit == nil || *runs.Limit != 0 {
				t.Errorf("%s/%s: autopilot_runs = %+v, want enforce 0", status, key, runs)
			}
			if runs.PeriodStart == nil || runs.PeriodEnd == nil || runs.ResetAt == nil {
				t.Errorf("%s/%s: the zero limit still needs its period", status, key)
			}
			if runs.Notifications != nil {
				t.Errorf("%s/%s: a blocked workspace gets no quota notice", status, key)
			}
			// The issue gate is the plan's, whatever the state.
			want := catalogue(t)[key].Limits["issue_count"]
			if g := gateOf(t, p, "issue_count"); (want == nil) != (g.Limit == nil) {
				t.Errorf("%s/%s: issue_count = %+v, plan limit %v", status, key, g, want)
			}
		}
	}

	active := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "active"}, now)
	if g := gateOf(t, active, "autopilot_runs"); *g.Limit != 50 {
		t.Errorf("active workspace autopilot_runs = %d, want 50", *g.Limit)
	}
}

// autopilot_runs resets on the UTC calendar month (D28).
func TestBuildAutopilotPeriodIsTheUTCCalendarMonth(t *testing.T) {
	cases := []struct {
		name       string
		now        string
		start, end string
	}{
		{"middle of a month", "2026-08-15T10:30:45Z", "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z"},
		{"first instant of a month", "2026-12-01T00:00:00Z", "2026-12-01T00:00:00Z", "2027-01-01T00:00:00Z"},
		{"last instant of a month", "2026-01-31T23:59:59.999999999Z", "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"},
		{"31 days into a 28-day month", "2027-02-28T23:59:59Z", "2027-02-01T00:00:00Z", "2027-03-01T00:00:00Z"},
		{"leap day", "2028-02-29T12:00:00Z", "2028-02-01T00:00:00Z", "2028-03-01T00:00:00Z"},
		{"year boundary, last second", "2026-12-31T23:59:59Z", "2026-12-01T00:00:00Z", "2027-01-01T00:00:00Z"},
		{"year boundary, first second", "2027-01-01T00:00:00Z", "2027-01-01T00:00:00Z", "2027-02-01T00:00:00Z"},
		// 23:30 on 31 March in UTC-5 is 04:30 on 1 April in UTC: the month is UTC's.
		{"offset instant on the boundary", "2026-03-31T23:30:00-05:00", "2026-04-01T00:00:00Z", "2026-05-01T00:00:00Z"},
		{"offset instant before the boundary", "2026-04-01T00:30:00+02:00", "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "active"}, at(t, tc.now))
			g := gateOf(t, p, "autopilot_runs")
			if g.PeriodStart == nil || g.PeriodEnd == nil || g.ResetAt == nil {
				t.Fatalf("period fields = %v %v %v, want all three", g.PeriodStart, g.PeriodEnd, g.ResetAt)
			}
			if !g.PeriodStart.Equal(at(t, tc.start)) || !g.PeriodEnd.Equal(at(t, tc.end)) {
				t.Errorf("period = [%s, %s), want [%s, %s)", g.PeriodStart.Format(time.RFC3339Nano), g.PeriodEnd.Format(time.RFC3339Nano), tc.start, tc.end)
			}
			if !g.ResetAt.Equal(*g.PeriodEnd) {
				t.Errorf("reset_at = %s, want period_end %s", g.ResetAt, g.PeriodEnd)
			}
			if !g.PeriodStart.Before(*g.PeriodEnd) || !g.PeriodStart.Before(*g.ResetAt) {
				t.Error("period_start must be before period_end and reset_at")
			}
			if g.PeriodStart.Location() != time.UTC {
				t.Errorf("period is in %v, want UTC", g.PeriodStart.Location())
			}
			if tn := at(t, tc.now); tn.Before(*g.PeriodStart) || !tn.Before(*g.PeriodEnd) {
				t.Errorf("now %s is outside its own period", tc.now)
			}
		})
	}
}

// An unlimited autopilot gate still names its period, as the plan requires.
func TestBuildAutopilotPeriodIsPresentWhenUnlimited(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], Overrides: plan.Limits{"autopilot_runs": nil}, WorkspaceStatus: "active"}, now)
	g := gateOf(t, p, "autopilot_runs")
	if g.Action != "off" || g.PeriodStart == nil || g.PeriodEnd == nil || g.ResetAt == nil {
		t.Errorf("autopilot_runs = %+v, want off with its period", g)
	}
}

// issue_count has no period: a partial period set would be an invalid gate.
func TestBuildIssueGateHasNoPeriod(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "active"}, now)
	g := gateOf(t, p, "issue_count")
	if g.PeriodStart != nil || g.PeriodEnd != nil || g.ResetAt != nil || g.Notifications != nil {
		t.Errorf("issue_count = %+v, want no period and no notice", g)
	}
}

func TestBuildAutopilotNoticeIsFirstRejectionPerPeriod(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "active"}, now)
	g := gateOf(t, p, "autopilot_runs")
	if g.Notifications == nil || g.Notifications.OnRejection != "first_rejection_per_period" {
		t.Errorf("notifications = %+v", g.Notifications)
	}
}

func TestBuildVersionIsTheSubscriptionVersion(t *testing.T) {
	for _, v := range []int64{0, 1, 2, 1 << 40} {
		if p := Build(Inputs{Plan: catalogue(t)["free"], Version: v, WorkspaceStatus: "active"}, now); p.SubscriptionVersion != v {
			t.Errorf("subscription_version = %d, want %d", p.SubscriptionVersion, v)
		}
	}
}

// The wire names are the client's: wirePolicy and wireGate in
// server/internal/entitlement/client.go.
func TestPolicyWireFieldNames(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], Version: 3, WorkspaceStatus: "active"}, now)
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema_version", "policy_revision", "subscription_version", "valid_until", "valid_for_seconds", "gates"} {
		if _, ok := top[k]; !ok {
			t.Errorf("policy has no %q", k)
		}
	}
	var gates map[string]map[string]json.RawMessage
	if err := json.Unmarshal(top["gates"], &gates); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"action", "limit", "period_start", "period_end", "reset_at", "notifications"} {
		if _, ok := gates["autopilot_runs"][k]; !ok {
			t.Errorf("autopilot_runs has no %q", k)
		}
	}
	var validUntil string
	if err := json.Unmarshal(top["valid_until"], &validUntil); err != nil || validUntil == "" {
		t.Errorf("valid_until = %s, want a non-empty RFC 3339 string", top["valid_until"])
	} else if _, err := time.Parse(time.RFC3339, validUntil); err != nil {
		t.Errorf("valid_until %q is not RFC 3339: %v", validUntil, err)
	}
}

// wantGateNames is every gate the policy carries: the two counted by the core
// since the first release, the three resource gates of spec 5.7, the two plan
// values, the four feature flags, and the member_groups count of spec 7b.2.
var wantGateNames = []string{
	"issue_count", "autopilot_runs",
	"agents", "runtimes", "storage_bytes",
	"api_rate_per_minute", "audit_retention_days",
	"sso", "workspace_policies", "viewer_role", "audit_export",
	"member_groups",
}

// A limit gate takes the plan's value, as an enforce gate; a plan value of
// "unlimited" is an off gate (spec 5.7).
func TestBuildLimitGatesTakeThePlansValue(t *testing.T) {
	for key, want := range map[string]map[string]int64{
		"free":     {"agents": 3, "runtimes": 2, "storage_bytes": 1 << 30, "api_rate_per_minute": 300, "audit_retention_days": 30},
		"pro":      {"agents": 30, "runtimes": 20, "storage_bytes": 20 << 30, "api_rate_per_minute": 1200, "audit_retention_days": 180},
		"business": {"agents": 200, "runtimes": 100, "storage_bytes": 200 << 30, "api_rate_per_minute": 3000, "audit_retention_days": 365},
	} {
		p := Build(Inputs{Plan: catalogue(t)[key], WorkspaceStatus: "active"}, now)
		for gate, limit := range want {
			g := gateOf(t, p, gate)
			if g.Action != "enforce" || g.Limit == nil || *g.Limit != limit {
				t.Errorf("%s %s = %+v, want enforce %d", key, gate, g, limit)
			}
			if g.PeriodStart != nil || g.PeriodEnd != nil || g.ResetAt != nil || g.Notifications != nil {
				t.Errorf("%s %s = %+v, want no period and no notice", key, gate, g)
			}
		}
	}
}

// A gate the plan leaves out has no limit, so it is off.
func TestBuildGateMissingFromThePlanIsOff(t *testing.T) {
	bare := plan.Plan{Key: "bare", Limits: plan.Limits{"issue_count": i64(10)}, Features: map[string]bool{}}
	p := Build(Inputs{Plan: bare, WorkspaceStatus: "active"}, now)
	for _, name := range []string{"agents", "runtimes", "storage_bytes", "audit_retention_days"} {
		if g := gateOf(t, p, name); g.Action != "off" || g.Limit != nil {
			t.Errorf("%s = %+v, want off with no limit", name, g)
		}
	}
	// An empty API rate on a limited plan is the ceiling (spec 7b.3 rule 2).
	if g := gateOf(t, p, "api_rate_per_minute"); g.Action != "enforce" || g.Limit == nil || *g.Limit != 6000 {
		t.Errorf("api_rate_per_minute = %+v, want enforce 6000", g)
	}

	// An explicit null in the plan is the same: unlimited.
	pro := catalogue(t)["pro"]
	pro.Limits = plan.Limits{"agents": nil}
	if g := gateOf(t, Build(Inputs{Plan: pro, WorkspaceStatus: "active"}, now), "agents"); g.Action != "off" || g.Limit != nil {
		t.Errorf("agents = %+v, want off", g)
	}
}

// An override replaces the plan's limit, and an empty one lifts it.
func TestBuildOverrideReplacesAResourceLimit(t *testing.T) {
	p := Build(Inputs{
		Plan: catalogue(t)["free"],
		Overrides: plan.Limits{
			"agents":        i64(10),
			"runtimes":      nil,
			"storage_bytes": i64(0),
		},
		WorkspaceStatus: "active",
	}, now)
	if g := gateOf(t, p, "agents"); g.Action != "enforce" || *g.Limit != 10 {
		t.Errorf("agents = %+v, want the override 10 over the plan's 3", g)
	}
	if g := gateOf(t, p, "runtimes"); g.Action != "off" || g.Limit != nil {
		t.Errorf("runtimes = %+v, want the empty override to lift the limit", g)
	}
	if g := gateOf(t, p, "storage_bytes"); g.Action != "enforce" || g.Limit == nil || *g.Limit != 0 {
		t.Errorf("storage_bytes = %+v, want an override of 0 to enforce 0", g)
	}
	if g := gateOf(t, p, "api_rate_per_minute"); *g.Limit != 300 {
		t.Errorf("api_rate_per_minute = %+v, want the plan's 300, which no override touched", g)
	}
}

// A flag that is on is off (allowed); a flag that is off is enforce 0 (refused).
func TestBuildFeatureFlags(t *testing.T) {
	flags := []string{"sso", "workspace_policies", "viewer_role", "audit_export"}
	cases := map[string]map[string]bool{
		"free":     {"audit_export": false, "sso": false, "workspace_policies": false, "viewer_role": false},
		"pro":      {"audit_export": true, "sso": false, "workspace_policies": false, "viewer_role": false},
		"business": {"audit_export": true, "sso": true, "workspace_policies": true, "viewer_role": true},
	}
	for key, want := range cases {
		p := Build(Inputs{Plan: catalogue(t)[key], WorkspaceStatus: "active"}, now)
		for _, f := range flags {
			g := gateOf(t, p, f)
			if want[f] {
				if g.Action != "off" || g.Limit != nil {
					t.Errorf("%s %s is on, gate = %+v, want off", key, f, g)
				}
				continue
			}
			if g.Action != "enforce" || g.Limit == nil || *g.Limit != 0 {
				t.Errorf("%s %s is off, gate = %+v, want enforce with limit 0", key, f, g)
			}
		}
	}
}

// A flag the plan does not mention is not included in the plan: it is refused.
func TestBuildFeatureFlagMissingFromThePlanIsRefused(t *testing.T) {
	bare := plan.Plan{Key: "bare", Limits: plan.Limits{}}
	p := Build(Inputs{Plan: bare, WorkspaceStatus: "active"}, now)
	for _, f := range []string{"sso", "workspace_policies", "viewer_role", "audit_export"} {
		if g := gateOf(t, p, f); g.Action != "enforce" || g.Limit == nil || *g.Limit != 0 {
			t.Errorf("%s = %+v, want enforce 0", f, g)
		}
	}
}

// Overrides carry limits only. They cannot switch a feature flag on.
func TestBuildOverridesDoNotTouchFeatureFlags(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], Overrides: plan.Limits{"sso": nil}, WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, "sso"); g.Action != "enforce" || *g.Limit != 0 {
		t.Errorf("sso = %+v, want enforce 0", g)
	}
}

// A workspace that cannot act keeps the plan's resource gates: only the
// autopilot gate is zeroed (spec 5.5).
func TestBuildNonActiveWorkspaceKeepsTheResourceGates(t *testing.T) {
	p := Build(Inputs{Plan: catalogue(t)["free"], WorkspaceStatus: "suspended"}, now)
	if g := gateOf(t, p, "agents"); g.Action != "enforce" || *g.Limit != 3 {
		t.Errorf("agents = %+v, want the plan's 3", g)
	}
}

// Every gate is valid on the client's rules: enforce has a non-negative limit,
// off has none.
func TestBuildEveryGateIsWellFormed(t *testing.T) {
	for key, pl := range catalogue(t) {
		p := Build(Inputs{Plan: pl, WorkspaceStatus: "active"}, now)
		for name, g := range p.Gates {
			switch g.Action {
			case "enforce":
				if g.Limit == nil || *g.Limit < 0 {
					t.Errorf("%s %s = %+v, enforce needs a non-negative limit", key, name, g)
				}
			case "off":
				if g.Limit != nil {
					t.Errorf("%s %s = %+v, off carries no limit", key, name, g)
				}
			default:
				t.Errorf("%s %s has action %q", key, name, g.Action)
			}
		}
	}
}

// mustPlan returns a catalogue plan and fails when the catalogue lacks it.
func mustPlan(t *testing.T, key string) plan.Plan {
	t.Helper()
	p, ok := catalogue(t)[key]
	if !ok {
		t.Fatalf("the catalogue has no plan %q", key)
	}
	return p
}

// The counted gates the policy serves (spec 7b.3 rule 1).
var countedGates = []string{
	plan.GateIssueCount, plan.GateAutopilotRuns, plan.GateAgents, plan.GateRuntimes, plan.GateStorageBytes,
	plan.GateMemberGroups,
}

func TestBuildUnlimitedPlanIsOffOnEveryCountedGate(t *testing.T) {
	p := Build(Inputs{Plan: mustPlan(t, "unlimited"), WorkspaceStatus: "active"}, now)
	for _, name := range countedGates {
		g := gateOf(t, p, name)
		if g.Action != ActionOff || g.Limit != nil {
			t.Errorf("%s = %+v, want off with no limit", name, g)
		}
	}
	if g := gateOf(t, p, plan.GateAutopilotRuns); g.Notifications != nil {
		t.Errorf("an off autopilot gate carries notifications: %+v", g.Notifications)
	}
}

func TestBuildPlanWithALimitKeepsItEvenNextToUnlimitedGates(t *testing.T) {
	pl := mustPlan(t, "unlimited")
	pl.Limits = plan.Limits{plan.GateAgents: i64(7)}
	p := Build(Inputs{Plan: pl, WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, plan.GateAgents); g.Action != ActionEnforce || g.Limit == nil || *g.Limit != 7 {
		t.Errorf("agents = %+v, want enforce 7", g)
	}
	if g := gateOf(t, p, plan.GateRuntimes); g.Action != ActionOff {
		t.Errorf("runtimes = %+v, want off", g)
	}
}

// Contract section 2: a plan whose api_rate_per_minute is empty has an off
// rate gate, not a ceiling number.
func TestBuildEmptyAPIRateIsOff(t *testing.T) {
	pl := mustPlan(t, "unlimited")
	pl.Limits = plan.Limits{plan.GateAPIRatePerMinute: nil}
	p := Build(Inputs{Plan: pl, WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, plan.GateAPIRatePerMinute); g.Action != ActionOff || g.Limit != nil {
		t.Errorf("api_rate_per_minute = %+v, want off with no limit", g)
	}
}

// Contract section 2: the unlimited plan's API rate cap is off, not a number.
func TestBuildUnlimitedPlanAPIRateIsOff(t *testing.T) {
	p := Build(Inputs{Plan: mustPlan(t, "unlimited"), WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, plan.GateAPIRatePerMinute); g.Action != ActionOff || g.Limit != nil {
		t.Errorf("api_rate_per_minute = %+v, want off with no limit", g)
	}
}

// A workspace that cannot act still has no runs on an unlimited plan (spec 7b.3 rule 3).
func TestBuildUnlimitedPlanDoesNotExemptANonActiveWorkspace(t *testing.T) {
	p := Build(Inputs{Plan: mustPlan(t, "unlimited"), WorkspaceStatus: "suspended"}, now)
	if g := gateOf(t, p, plan.GateAutopilotRuns); g.Action != ActionEnforce || g.Limit == nil || *g.Limit != 0 {
		t.Errorf("autopilot_runs = %+v, want enforce 0", g)
	}
}

// Moving off the unlimited plan applies the new limits at once; nothing else
// in the policy depends on the old plan.
func TestBuildAfterDowngradeFromUnlimitedUsesTheNewLimits(t *testing.T) {
	cat := catalogue(t)
	before := Build(Inputs{Plan: mustPlan(t, "unlimited"), Version: 1, WorkspaceStatus: "active"}, now)
	after := Build(Inputs{Plan: cat["free"], Version: 2, WorkspaceStatus: "active"}, now)
	if gateOf(t, before, plan.GateAgents).Action != ActionOff {
		t.Fatal("unlimited agents gate is not off")
	}
	if g := gateOf(t, after, plan.GateAgents); g.Action != ActionEnforce || *g.Limit != 3 {
		t.Errorf("free agents = %+v, want enforce 3", g)
	}
}

// The four plans of spec 5.5 produce the policy they produced before Q8.
func TestBuildGoldenForTheFourOriginalPlans(t *testing.T) {
	raw, err := os.ReadFile("testdata/policy_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]json.RawMessage
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"free", "pro", "business", "enterprise"} {
		got, err := json.Marshal(Build(Inputs{Plan: catalogue(t)[key], Version: 3, WorkspaceStatus: "active"}, now))
		if err != nil {
			t.Fatal(err)
		}
		var a, b any
		_ = json.Unmarshal(got, &a)
		_ = json.Unmarshal(want[key], &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s policy changed\n got: %s\nwant: %s", key, got, want[key])
		}
	}
}

// Contract section 2 and spec 7b.3 rule 2: the ceiling applies to an empty
// API rate on a limited plan; the unlimited plan's cap stays off.
func TestBuildLimitedPlanWithEmptyAPIRateUsesTheCeiling(t *testing.T) {
	pl := mustPlan(t, "pro")
	pl.Limits = plan.Limits{plan.GateIssueCount: i64(5), plan.GateAPIRatePerMinute: nil}
	p := Build(Inputs{Plan: pl, WorkspaceStatus: "active", APIRateCeiling: 5000}, now)
	if g := gateOf(t, p, plan.GateAPIRatePerMinute); g.Action != ActionEnforce || g.Limit == nil || *g.Limit != 5000 {
		t.Errorf("api_rate_per_minute = %+v, want enforce 5000", g)
	}
}

func TestBuildAPIRateCeilingDefaultsTo6000(t *testing.T) {
	pl := mustPlan(t, "pro")
	pl.Limits = plan.Limits{plan.GateIssueCount: i64(5), plan.GateAPIRatePerMinute: nil}
	p := Build(Inputs{Plan: pl, WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, plan.GateAPIRatePerMinute); g.Action != ActionEnforce || g.Limit == nil || *g.Limit != 6000 {
		t.Errorf("api_rate_per_minute = %+v, want enforce 6000", g)
	}
}

func TestBuildUnlimitedPlanCapStaysOffWithACeiling(t *testing.T) {
	p := Build(Inputs{Plan: mustPlan(t, "unlimited"), WorkspaceStatus: "active", APIRateCeiling: 5000}, now)
	if g := gateOf(t, p, plan.GateAPIRatePerMinute); g.Action != ActionOff || g.Limit != nil {
		t.Errorf("api_rate_per_minute = %+v, want off", g)
	}
}

// A plan's own API rate is used as it is; the ceiling only replaces an empty one.
func TestBuildPlanAPIRateIsNotCappedByTheCeiling(t *testing.T) {
	p := Build(Inputs{Plan: mustPlan(t, "pro"), WorkspaceStatus: "active", APIRateCeiling: 500}, now)
	if g := gateOf(t, p, plan.GateAPIRatePerMinute); g.Action != ActionEnforce || g.Limit == nil || *g.Limit != 1200 {
		t.Errorf("api_rate_per_minute = %+v, want enforce 1200", g)
	}
}

// The member_groups gate of spec 7b.2 rule 12: Free and Pro include no groups
// (enforce 0), Business caps them at 50, Enterprise and the unlimited plan have
// no cap (off).
func TestBuildMemberGroupsGateFollowsThePlan(t *testing.T) {
	for key, want := range map[string]*int64{
		"free": i64(0), "pro": i64(0), "business": i64(50), "enterprise": nil, "unlimited": nil,
	} {
		p := Build(Inputs{Plan: mustPlan(t, key), WorkspaceStatus: "active"}, now)
		g := gateOf(t, p, "member_groups")
		if want == nil {
			if g.Action != ActionOff || g.Limit != nil {
				t.Errorf("%s member_groups = %+v, want off", key, g)
			}
		} else if g.Action != ActionEnforce || g.Limit == nil || *g.Limit != *want {
			t.Errorf("%s member_groups = %+v, want enforce %d", key, g, *want)
		}
		if g.PeriodStart != nil || g.PeriodEnd != nil || g.ResetAt != nil || g.Notifications != nil {
			t.Errorf("%s member_groups = %+v, want no period and no notice", key, g)
		}
	}
}

// A workspace override replaces the plan's cap, and an empty one lifts it.
func TestBuildMemberGroupsGateTakesOverrides(t *testing.T) {
	free := mustPlan(t, "free")
	p := Build(Inputs{Plan: free, Overrides: plan.Limits{plan.GateMemberGroups: i64(10)}, WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, "member_groups"); g.Action != ActionEnforce || *g.Limit != 10 {
		t.Errorf("override 10: %+v", g)
	}
	p = Build(Inputs{Plan: free, Overrides: plan.Limits{plan.GateMemberGroups: nil}, WorkspaceStatus: "active"}, now)
	if g := gateOf(t, p, "member_groups"); g.Action != ActionOff || g.Limit != nil {
		t.Errorf("empty override: %+v", g)
	}
}

// A suspended workspace keeps its member_groups cap: only autopilot is zeroed.
func TestBuildNonActiveWorkspaceKeepsTheMemberGroupsGate(t *testing.T) {
	p := Build(Inputs{Plan: mustPlan(t, "business"), WorkspaceStatus: "suspended"}, now)
	if g := gateOf(t, p, "member_groups"); g.Action != ActionEnforce || *g.Limit != 50 {
		t.Errorf("member_groups = %+v, want enforce 50", g)
	}
}
