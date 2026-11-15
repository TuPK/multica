// Package entitlement serves the entitlement policy that Multica's existing
// client reads (spec 5.5). The wire format is the client's wirePolicy in
// server/internal/entitlement/client.go; the contract tests in
// platform-management/contract feed this package's output through that code.
package entitlement

import (
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const (
	// SchemaVersion is the only wire version the client accepts.
	SchemaVersion = 1
	// PolicyRevision is the generation of the instruction set: 2 added the
	// autopilot notification policy. It is a constant of this build; the client
	// only needs it positive.
	PolicyRevision = 2
	// ValidFor is how long the client may use a policy, and how long this
	// service caches its inputs. The client accepts 1 to 300 seconds.
	ValidFor = 30 * time.Second

	// NotifyFirstRejectionPerPeriod is the one notice policy the client knows.
	NotifyFirstRejectionPerPeriod = "first_rejection_per_period"
)

// Wire actions.
const (
	ActionEnforce = "enforce"
	ActionOff     = "off"
)

// Policy is the response body.
type Policy struct {
	SchemaVersion       int             `json:"schema_version"`
	PolicyRevision      int64           `json:"policy_revision"`
	SubscriptionVersion int64           `json:"subscription_version"`
	ValidUntil          time.Time       `json:"valid_until"`
	ValidForSeconds     int64           `json:"valid_for_seconds"`
	Gates               map[string]Gate `json:"gates"`
}

// Gate is one gate's instruction. An off gate carries only its action.
type Gate struct {
	Action        string         `json:"action"`
	Limit         *int64         `json:"limit,omitempty"`
	PeriodStart   *time.Time     `json:"period_start,omitempty"`
	PeriodEnd     *time.Time     `json:"period_end,omitempty"`
	ResetAt       *time.Time     `json:"reset_at,omitempty"`
	Notifications *Notifications `json:"notifications,omitempty"`
}

// Notifications is the additive notice policy of a gate.
type Notifications struct {
	OnRejection string `json:"on_rejection"`
}

// Inputs are the facts a policy is built from.
type Inputs struct {
	// Plan is the workspace's plan: the free plan when it has no subscription row.
	Plan plan.Plan
	// Version is the subscription's version, 0 with no row.
	Version int64
	// Overrides are the workspace's limit_overrides.
	Overrides plan.Limits
	// WorkspaceStatus is the platform_workspace status: "active" with no row.
	WorkspaceStatus string
}

// Build returns the policy for in at the instant now. It is pure: the same
// inputs and instant give the same policy.
func Build(in Inputs, now time.Time) Policy {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	issues := gate(plan.EffectiveLimit(in.Plan.Limits, in.Overrides, plan.GateIssueCount))

	runs := gate(plan.EffectiveLimit(in.Plan.Limits, in.Overrides, plan.GateAutopilotRuns))
	active := in.WorkspaceStatus == "active"
	if !active {
		// A workspace that cannot act may not start runs, whatever its plan says
		// (spec 5.5). A zero limit is a valid enforce gate, and the client
		// rejects every run.
		zero := int64(0)
		runs = Gate{Action: ActionEnforce, Limit: &zero}
	}
	runs.PeriodStart, runs.PeriodEnd, runs.ResetAt = &start, &end, &end
	if runs.Action == ActionEnforce && active {
		runs.Notifications = &Notifications{OnRejection: NotifyFirstRejectionPerPeriod}
	}

	return Policy{
		SchemaVersion:       SchemaVersion,
		PolicyRevision:      PolicyRevision,
		SubscriptionVersion: in.Version,
		ValidUntil:          now.Truncate(time.Second).Add(ValidFor),
		ValidForSeconds:     int64(ValidFor / time.Second),
		Gates: map[string]Gate{
			plan.GateIssueCount:    issues,
			plan.GateAutopilotRuns: runs,
		},
	}
}

// gate turns an effective limit into an instruction: no limit is off.
func gate(limit *int64) Gate {
	if limit == nil {
		return Gate{Action: ActionOff}
	}
	v := *limit
	return Gate{Action: ActionEnforce, Limit: &v}
}
