package entitlement

import (
	"context"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Sibling is one workspace of a tenant with the plan it is on and its limit
// overrides.
type Sibling struct {
	Plan      plan.Plan
	Overrides plan.Limits
}

// TenantMaxWorkspaces returns the max_workspaces limit of a tenant: the largest
// effective value among its workspaces, and nil (no limit) when any workspace
// has none (spec R6 section 5.3, plan A).
func TenantMaxWorkspaces(siblings []Sibling) *int64 {
	var largest int64
	for _, s := range siblings {
		limit := plan.EffectiveLimit(s.Plan.Limits, s.Overrides, plan.GateMaxWorkspaces)
		if limit == nil {
			return nil
		}
		largest = max(largest, *limit)
	}
	return &largest
}

// TenantReader lists the workspaces that share a tenant.
type TenantReader interface {
	// SiblingIDs returns the ids of every workspace in the tenant of workspaceID,
	// itself included. A workspace with no tenant link gives none.
	SiblingIDs(ctx context.Context, workspaceID string) ([]string, error)
}
