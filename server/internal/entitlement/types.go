package entitlement

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const SchemaVersion = 1

type GateName string

const (
	GateIssueCount    GateName = "issue_count"
	GateAutopilotRuns GateName = "autopilot_runs"

	// Resource gates, counted by the enforcement point when it creates the
	// resource.
	GateAgents       GateName = "agents"
	GateRuntimes     GateName = "runtimes"
	GateStorageBytes GateName = "storage_bytes"

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
// policy invalid.
var requiredGates = []GateName{GateIssueCount, GateAutopilotRuns}

// optionalGates may be absent: Cloud and older platform builds do not send
// them, and an absent gate is `off`.
var optionalGates = []GateName{
	GateAgents, GateRuntimes, GateStorageBytes,
	GateAPIRatePerMinute, GateAuditRetentionDays,
	GateSSO, GateWorkspacePolicies, GateViewerRole, GateAuditExport,
	GateMemberGroups, GateMaxWorkspaces,
}

func (n GateName) valid() bool {
	switch n {
	case GateIssueCount, GateAutopilotRuns,
		GateAgents, GateRuntimes, GateStorageBytes,
		GateAPIRatePerMinute, GateAuditRetentionDays,
		GateSSO, GateWorkspacePolicies, GateViewerRole, GateAuditExport,
		GateMemberGroups, GateMaxWorkspaces:
		return true
	default:
		return false
	}
}

type Action string

const (
	ActionOff     Action = "off"
	ActionObserve Action = "observe"
	ActionEnforce Action = "enforce"

	NotificationFirstRejectionPerPeriod = "first_rejection_per_period"
)

type Reason string

const (
	ReasonDisabled          Reason = "disabled"
	ReasonInvalidWorkspace  Reason = "invalid_workspace"
	ReasonUnknownGate       Reason = "unknown_gate"
	ReasonCacheFresh        Reason = "cache_fresh"
	ReasonRefreshed         Reason = "refreshed"
	ReasonStale             Reason = "stale"
	ReasonUnavailable       Reason = "unavailable"
	ReasonInvalidPolicy     Reason = "invalid_policy"
	ReasonVersionRegression Reason = "version_regression"
	ReasonStub              Reason = "stub"
)

// Gate is the effective instruction for one generic enforcement point. Limits
// and period boundaries come from Cloud; this package does not derive them.
type Gate struct {
	Action        Action
	Limit         *int
	PeriodStart   *time.Time
	PeriodEnd     *time.Time
	ResetAt       *time.Time
	Notifications *NotificationPolicy
}

type NotificationPolicy struct {
	OnRejection string
}

// Decision carries enough source information for consumers to audit why a gate
// was enforced, observed, or disabled without exposing Cloud's private inputs.
type Decision struct {
	Gate                Gate
	Reason              Reason
	PolicyRevision      int64
	SubscriptionVersion int64
	CloudValidUntil     time.Time
}

// Provider is the only interface issue-count and autopilot consumers
// need. It deliberately has no error return: every failure is represented by a
// fail-open Decision with ActionOff (or ActionObserve for a bounded stale
// snapshot that can no longer enforce).
type Provider interface {
	Gate(ctx context.Context, workspaceID uuid.UUID, name GateName) Decision
}

// Observer is a low-cardinality instrumentation seam. Implementations must not
// attach workspace IDs or policy values as metric labels.
type Observer interface {
	RecordEntitlementCache(outcome string)
	RecordEntitlementRefresh(outcome string, durationSeconds float64)
	RecordEntitlementDecision(gate, action, reason string)
	RecordEntitlementVersionRegression()
}

func offDecision(reason Reason) Decision {
	return Decision{Gate: Gate{Action: ActionOff}, Reason: reason}
}

func cloneDecision(in Decision) Decision {
	in.Gate = cloneGate(in.Gate)
	return in
}

func cloneGate(in Gate) Gate {
	out := in
	if in.Limit != nil {
		value := *in.Limit
		out.Limit = &value
	}
	if in.PeriodStart != nil {
		value := *in.PeriodStart
		out.PeriodStart = &value
	}
	if in.PeriodEnd != nil {
		value := *in.PeriodEnd
		out.PeriodEnd = &value
	}
	if in.ResetAt != nil {
		value := *in.ResetAt
		out.ResetAt = &value
	}
	if in.Notifications != nil {
		out.Notifications = &NotificationPolicy{
			OnRejection: in.Notifications.OnRejection,
		}
	}
	return out
}
