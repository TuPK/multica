package entitlement

// The fork's gates. Upstream owns types.go and client.go; they reach this file
// through forkGateValid and normalizeForkGates only, so an upstream edit of its
// own gate list does not conflict with the fork's.

const (
	// GateIssueCount limits the issue rows of a workspace. Upstream removed
	// it; the fork's platform still serves it.
	GateIssueCount GateName = "issue_count"

	// Resource gates, counted by the enforcement point when it creates the
	// resource.
	GateAgents   GateName = "agents"
	GateRuntimes GateName = "runtimes"

	// Plan values. The API rate limit and the audit retention job read the
	// limit; the feature flags below are `off` when the plan includes the
	// feature and `enforce` with limit 0 when it does not.
	GateAPIRatePerMinute   GateName = "api_rate_per_minute"
	GateAuditRetentionDays GateName = "audit_retention_days"
	GateSSO                GateName = "sso"
	GateWorkspacePolicies  GateName = "workspace_policies"
	GateViewerRole         GateName = "viewer_role"
	GateAuditExport        GateName = "audit_export"

	// GateMemberGroups caps the account groups of a workspace (spec 7b.2). The
	// group routes count it when a group is created.
	GateMemberGroups GateName = "member_groups"

	// GateMaxWorkspaces caps the workspaces of a tenant (spec R6 section 5.7).
	// The platform serves the tenant's limit in the policy of every workspace of
	// the tenant; CreateWorkspace counts it when a workspace joins a tenant.
	GateMaxWorkspaces GateName = "max_workspaces"
)

// requiredGates must be present in every policy. An absent one makes the
// policy invalid. Upstream's normalizePolicy reads autopilot_runs itself.
var requiredGates = []GateName{GateAutopilotRuns}

// optionalGates are the fork's added gates. Cloud and older platform builds do
// not send them, and an absent gate is `off`. A malformed one invalidates the
// policy.
var optionalGates = []GateName{
	GateIssueCount, GateAgents, GateRuntimes,
	GateAPIRatePerMinute, GateAuditRetentionDays,
	GateSSO, GateWorkspacePolicies, GateViewerRole, GateAuditExport,
	GateMemberGroups, GateMaxWorkspaces,
}

func forkGateValid(n GateName) bool {
	for _, g := range optionalGates {
		if g == n {
			return true
		}
	}
	return false
}

// normalizeForkGates reads the fork's gates from a wire policy into gates. A
// gate name this client does not know is never read, so an older client parses
// a newer policy. A known optional gate that is absent is off.
func normalizeForkGates(wire wirePolicy, gates map[GateName]Gate) error {
	for _, name := range optionalGates {
		wireGate, ok := wire.Gates[string(name)]
		if !ok {
			gates[name] = Gate{Action: ActionOff}
			continue
		}
		gate, err := normalizeGate(name, wireGate)
		if err != nil {
			return err
		}
		gates[name] = gate
	}
	return nil
}
