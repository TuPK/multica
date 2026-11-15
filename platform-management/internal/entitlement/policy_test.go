package entitlement

import (
	"encoding/json"
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
	if len(p.Gates) != 2 {
		t.Errorf("gates = %d, want only the two the client reads", len(p.Gates))
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
		Overrides:       plan.Limits{"agents": i64(5)}, // a gate the policy does not carry
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
