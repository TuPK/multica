package entitlement

import (
	"context"
	"testing"
)

func TestNormalizePolicyAcceptsMaxWorkspacesGate(t *testing.T) {
	if GateMaxWorkspaces != "max_workspaces" || !GateMaxWorkspaces.valid() {
		t.Fatalf("GateMaxWorkspaces = %q valid=%v, want the wire name max_workspaces and a valid gate", GateMaxWorkspaces, GateMaxWorkspaces.valid())
	}
	limit := 3
	policy := samplePolicy(2, 4, 60, ActionEnforce)
	policy.Gates["max_workspaces"] = wireGate{Action: "enforce", Limit: &limit}
	client, ws := clientFor(t, policy)
	d := client.Gate(context.Background(), ws, GateMaxWorkspaces)
	if d.Gate.Action != ActionEnforce || d.Gate.Limit == nil || *d.Gate.Limit != 3 || d.Reason == ReasonInvalidPolicy || d.Reason == ReasonUnknownGate {
		t.Fatalf("max_workspaces = %+v, want enforce 3 from the policy", d)
	}

	policy.Gates["max_workspaces"] = wireGate{Action: "off"}
	client, ws = clientFor(t, policy)
	if d := client.Gate(context.Background(), ws, GateMaxWorkspaces); d.Gate.Action != ActionOff || d.Gate.Limit != nil || d.Reason == ReasonInvalidPolicy {
		t.Fatalf("max_workspaces = %+v, want off", d)
	}
}

// Delegora Cloud and older platform builds send no max_workspaces gate. Their
// policy stays valid and the gate is off.
func TestAbsentMaxWorkspacesGateIsOff(t *testing.T) {
	policy := samplePolicy(2, 4, 60, ActionEnforce)
	if _, ok := policy.Gates["max_workspaces"]; ok {
		t.Fatal("the sample policy already has the gate")
	}
	client, ws := clientFor(t, policy)
	d := client.Gate(context.Background(), ws, GateMaxWorkspaces)
	if d.Gate.Action != ActionOff || d.Gate.Limit != nil || d.Reason == ReasonInvalidPolicy || d.Reason == ReasonUnknownGate {
		t.Fatalf("max_workspaces = %+v, want off with the policy accepted", d)
	}
	// The policy itself was accepted: its other gates are served.
	if d := client.Gate(context.Background(), ws, GateIssueCount); d.Gate.Action != ActionEnforce {
		t.Fatalf("issue_count = %+v, want the policy's enforce", d)
	}
}

func TestUnknownGateIsStillRejected(t *testing.T) {
	client, ws := clientFor(t, samplePolicy(2, 4, 60, ActionEnforce))
	d := client.Gate(context.Background(), ws, GateName("max_workspaces_typo"))
	if d.Gate.Action != ActionOff || d.Reason != ReasonUnknownGate {
		t.Fatalf("unknown gate = %+v, want off/unknown_gate", d)
	}
}
