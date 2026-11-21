package entitlement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// The gates of spec 5.7 beyond the two the core has always counted. Resource
// gates are counted at creation; the rest carry a value of the plan.
var addedGates = []GateName{
	GateAgents, GateRuntimes, GateStorageBytes,
	GateAPIRatePerMinute, GateAuditRetentionDays,
	GateSSO, GateWorkspacePolicies, GateViewerRole, GateAuditExport,
	GateMemberGroups,
}

func TestAddedGateNamesAreTheWireNames(t *testing.T) {
	want := map[GateName]string{
		GateAgents: "agents", GateRuntimes: "runtimes", GateStorageBytes: "storage_bytes",
		GateAPIRatePerMinute: "api_rate_per_minute", GateAuditRetentionDays: "audit_retention_days",
		GateSSO: "sso", GateWorkspacePolicies: "workspace_policies", GateViewerRole: "viewer_role",
		GateAuditExport: "audit_export", GateMemberGroups: "member_groups",
	}
	if len(want) != len(addedGates) {
		t.Fatalf("test covers %d names, addedGates has %d", len(want), len(addedGates))
	}
	for gate, name := range want {
		if string(gate) != name {
			t.Errorf("%v = %q, want %q", gate, string(gate), name)
		}
		if !gate.valid() {
			t.Errorf("%s is not a valid gate", name)
		}
	}
}

func policyWithAddedGates() wirePolicy {
	policy := samplePolicy(2, 4, 60, ActionEnforce)
	limit := func(n int) *int { return &n }
	policy.Gates["agents"] = wireGate{Action: "enforce", Limit: limit(30)}
	policy.Gates["runtimes"] = wireGate{Action: "enforce", Limit: limit(20)}
	policy.Gates["storage_bytes"] = wireGate{Action: "enforce", Limit: limit(21474836480)}
	policy.Gates["api_rate_per_minute"] = wireGate{Action: "enforce", Limit: limit(1200)}
	policy.Gates["audit_retention_days"] = wireGate{Action: "enforce", Limit: limit(180)}
	policy.Gates["audit_export"] = wireGate{Action: "off"}
	policy.Gates["sso"] = wireGate{Action: "enforce", Limit: limit(0)}
	policy.Gates["workspace_policies"] = wireGate{Action: "enforce", Limit: limit(0)}
	policy.Gates["viewer_role"] = wireGate{Action: "off"}
	policy.Gates["member_groups"] = wireGate{Action: "enforce", Limit: limit(50)}
	return policy
}

func clientFor(t *testing.T, policy wirePolicy) (*Client, uuid.UUID) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writePolicy(t, w, policy)
	}))
	t.Cleanup(server.Close)
	return newTestClient(t, server.URL, nil), uuid.New()
}

func TestPolicyWithAddedGatesParses(t *testing.T) {
	client, ws := clientFor(t, policyWithAddedGates())
	ctx := context.Background()

	for gate, want := range map[GateName]int{
		GateAgents: 30, GateRuntimes: 20, GateStorageBytes: 21474836480,
		GateAPIRatePerMinute: 1200, GateAuditRetentionDays: 180,
		GateSSO: 0, GateWorkspacePolicies: 0, GateMemberGroups: 50,
	} {
		d := client.Gate(ctx, ws, gate)
		if d.Gate.Action != ActionEnforce || d.Gate.Limit == nil || *d.Gate.Limit != want {
			t.Errorf("%s = %+v, want enforce %d", gate, d, want)
		}
		if d.PolicyRevision != 2 || d.SubscriptionVersion != 4 {
			t.Errorf("%s revision/version = %d/%d, want 2/4", gate, d.PolicyRevision, d.SubscriptionVersion)
		}
	}
	for _, gate := range []GateName{GateAuditExport, GateViewerRole} {
		if d := client.Gate(ctx, ws, gate); d.Gate.Action != ActionOff || d.Gate.Limit != nil {
			t.Errorf("%s = %+v, want off", gate, d)
		}
	}
	// The two original gates are unchanged by the added ones.
	if d := client.Gate(ctx, ws, GateIssueCount); d.Gate.Action != ActionEnforce || *d.Gate.Limit != testIssueLimit {
		t.Errorf("issue_count = %+v", d)
	}
}

// Added gates are decided from the one cached policy: no second request.
func TestAddedGatesShareTheWorkspacesCachedPolicy(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		writePolicy(t, w, policyWithAddedGates())
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)
	ws := uuid.New()
	for _, gate := range append([]GateName{GateIssueCount, GateAutopilotRuns}, addedGates...) {
		client.Gate(context.Background(), ws, gate)
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1", hits)
	}
}

func TestAddedGatesAreOffInAPolicyThatLacksThem(t *testing.T) {
	// samplePolicy is what Multica Cloud and an older platform serve: two gates.
	client, ws := clientFor(t, samplePolicy(1, 0, 60, ActionEnforce))
	for _, gate := range addedGates {
		d := client.Gate(context.Background(), ws, gate)
		if d.Gate.Action != ActionOff || d.Gate.Limit != nil || d.Gate.PeriodStart != nil {
			t.Errorf("%s = %+v, want off with no limit", gate, d.Gate)
		}
		if d.Reason != ReasonRefreshed && d.Reason != ReasonCacheFresh && d.Reason != ReasonGateMissing {
			t.Errorf("%s reason = %s, want the policy's own (not unknown_gate or invalid_policy)", gate, d.Reason)
		}
		if d.PolicyRevision != 1 {
			t.Errorf("%s policy revision = %d, want the policy's 1", gate, d.PolicyRevision)
		}
	}
	// And the two enforced gates still enforce.
	if d := client.Gate(context.Background(), ws, GateIssueCount); d.Gate.Action != ActionEnforce {
		t.Errorf("issue_count = %+v", d)
	}
}

func TestUnknownGateInAPolicyIsIgnored(t *testing.T) {
	policy := policyWithAddedGates()
	limit := 9
	policy.Gates["gate_from_the_future"] = wireGate{Action: "enforce", Limit: &limit}
	// Not even a shape this client knows: it must not invalidate the policy.
	policy.Gates["another_future_gate"] = wireGate{Action: "observe"}

	var wire wirePolicy
	body, _ := json.Marshal(policy)
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	fetched, err := normalizePolicy(wire)
	if err != nil {
		t.Fatalf("normalizePolicy: %v", err)
	}
	for name := range fetched.snapshot.gates {
		if !name.valid() {
			t.Errorf("snapshot holds unknown gate %q", name)
		}
	}

	client, ws := clientFor(t, policy)
	if d := client.Gate(context.Background(), ws, GateAgents); d.Gate.Action != ActionEnforce || *d.Gate.Limit != 30 {
		t.Errorf("agents = %+v, want the policy's gate despite the unknown ones", d)
	}
	// Asking for the unknown name is still a refusal to fetch: it stays off.
	if d := client.Gate(context.Background(), ws, GateName("gate_from_the_future")); d.Gate.Action != ActionOff || d.Reason != ReasonUnknownGate {
		t.Errorf("unknown gate = %+v, want off/unknown_gate", d)
	}
}

// A malformed gate of a name the client knows is a malformed policy, as it
// always was for the two original gates.
func TestMalformedAddedGateInvalidatesThePolicy(t *testing.T) {
	negative := -1
	cases := map[string]wireGate{
		"enforce without a limit": {Action: "enforce"},
		"negative limit":          {Action: "enforce", Limit: &negative},
		"unknown action":          {Action: "observe"},
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			policy := policyWithAddedGates()
			policy.Gates["agents"] = g
			var wire wirePolicy
			body, _ := json.Marshal(policy)
			if err := json.Unmarshal(body, &wire); err != nil {
				t.Fatal(err)
			}
			if _, err := normalizePolicy(wire); !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("normalizePolicy = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

// autopilot_runs stays required; issue_count is optional because upstream
// removed it, so a policy without it is valid and the gate is off.
func TestAutopilotGateRemainsRequired(t *testing.T) {
	policy := policyWithAddedGates()
	delete(policy.Gates, "autopilot_runs")
	var wire wirePolicy
	body, _ := json.Marshal(policy)
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizePolicy(wire); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("normalizePolicy = %v, want ErrInvalidPolicy", err)
	}
}

func TestPolicyWithoutIssueCountIsValid(t *testing.T) {
	policy := policyWithAddedGates()
	delete(policy.Gates, "issue_count")
	var wire wirePolicy
	body, _ := json.Marshal(policy)
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	fetched, err := normalizePolicy(wire)
	if err != nil {
		t.Fatalf("normalizePolicy = %v, want a valid policy", err)
	}
	if fetched.snapshot.gates[GateIssueCount].Action != ActionOff {
		t.Fatalf("issue_count = %+v, want off", fetched.snapshot.gates[GateIssueCount])
	}
}

// An added gate never has a period, but a complete one is accepted: that is
// the existing rule for any gate but autopilot_runs.
func TestAddedGateKeepsTheSharedPeriodRule(t *testing.T) {
	policy := policyWithAddedGates()
	start := policy.Gates["autopilot_runs"].PeriodStart
	policy.Gates["agents"] = wireGate{Action: "enforce", Limit: new(int), PeriodStart: start}
	var wire wirePolicy
	body, _ := json.Marshal(policy)
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizePolicy(wire); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("a partial period: normalizePolicy = %v, want ErrInvalidPolicy", err)
	}
}

func TestAddedGatesAreOffWhenDisabled(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range addedGates {
		d := client.Gate(context.Background(), uuid.New(), gate)
		if d.Gate.Action != ActionOff || d.Reason != ReasonDisabled {
			t.Errorf("%s = %+v, want off/disabled", gate, d)
		}
	}
}

// member_groups is a counted gate of spec 7b.2: a plan that includes no groups
// is `enforce` with limit 0, a plan with a cap is `enforce` with the cap, and an
// unlimited plan is `off`.
func TestMemberGroupsGateIsParsedFromThePolicy(t *testing.T) {
	if !GateMemberGroups.valid() || string(GateMemberGroups) != "member_groups" {
		t.Fatalf("GateMemberGroups = %q, valid %v", GateMemberGroups, GateMemberGroups.valid())
	}
	found := false
	for _, name := range optionalGates {
		found = found || name == GateMemberGroups
	}
	if !found {
		t.Fatal("member_groups is not an optional gate: a policy without it would be invalid or the gate never read")
	}

	zero, fifty := 0, 50
	for name, tc := range map[string]struct {
		gate  wireGate
		limit *int
		off   bool
	}{
		"none included": {gate: wireGate{Action: "enforce", Limit: &zero}, limit: &zero},
		"capped":        {gate: wireGate{Action: "enforce", Limit: &fifty}, limit: &fifty},
		"unlimited":     {gate: wireGate{Action: "off"}, off: true},
	} {
		t.Run(name, func(t *testing.T) {
			policy := policyWithAddedGates()
			policy.Gates["member_groups"] = tc.gate
			client, ws := clientFor(t, policy)
			d := client.Gate(context.Background(), ws, GateMemberGroups)
			if tc.off {
				if d.Gate.Action != ActionOff || d.Gate.Limit != nil {
					t.Fatalf("member_groups = %+v, want off", d)
				}
				return
			}
			if d.Gate.Action != ActionEnforce || d.Gate.Limit == nil || *d.Gate.Limit != *tc.limit {
				t.Fatalf("member_groups = %+v, want enforce %d", d, *tc.limit)
			}
			if d.Reason != ReasonRefreshed && d.Reason != ReasonCacheFresh && d.Reason != ReasonGateMissing {
				t.Errorf("reason = %s", d.Reason)
			}
		})
	}
}

// A policy from an older platform or Cloud has no member_groups gate; it is
// off, so the number of groups is not limited.
func TestMemberGroupsGateAbsentIsOff(t *testing.T) {
	client, ws := clientFor(t, samplePolicy(1, 0, 60, ActionEnforce))
	d := client.Gate(context.Background(), ws, GateMemberGroups)
	if d.Gate.Action != ActionOff || d.Gate.Limit != nil || d.Reason == ReasonUnknownGate || d.Reason == ReasonInvalidPolicy {
		t.Fatalf("member_groups = %+v, want off with the policy's own reason", d)
	}
}

func TestMemberGroupsGateMalformedInvalidatesThePolicy(t *testing.T) {
	negative := -1
	for name, g := range map[string]wireGate{
		"enforce without a limit": {Action: "enforce"},
		"negative limit":          {Action: "enforce", Limit: &negative},
		"unknown action":          {Action: "observe"},
	} {
		t.Run(name, func(t *testing.T) {
			policy := policyWithAddedGates()
			policy.Gates["member_groups"] = g
			var wire wirePolicy
			body, _ := json.Marshal(policy)
			if err := json.Unmarshal(body, &wire); err != nil {
				t.Fatal(err)
			}
			if _, err := normalizePolicy(wire); !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("normalizePolicy = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

func TestMemberGroupsGateIsOffWhenDisabled(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	d := client.Gate(context.Background(), uuid.New(), GateMemberGroups)
	if d.Gate.Action != ActionOff || d.Reason != ReasonDisabled {
		t.Fatalf("member_groups = %+v, want off/disabled", d)
	}
}
