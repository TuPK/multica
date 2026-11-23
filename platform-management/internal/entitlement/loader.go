package entitlement

import (
	"context"
	"fmt"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// StatusReader reads a workspace's platform status.
type StatusReader interface {
	// Status returns "active" for a workspace with no platform row.
	Status(ctx context.Context, workspaceID string) (string, error)
}

// Loader assembles Inputs from the plan, subscription and workspace-state
// tables.
type Loader struct {
	Plans         plan.Reader
	Subscriptions subscription.Reader
	Status        StatusReader
	// Tenants gives the workspaces of a workspace's tenant. Nil means no tenant
	// data: the max_workspaces gate is off.
	Tenants TenantReader
}

// Inputs implements Source. A workspace with no subscription row is on the free
// plan at version 0. A subscription on a plan that no longer exists is an
// error: the caller answers 5xx, which Delegora reads as "platform unavailable",
// rather than guessing a plan.
func (l Loader) Inputs(ctx context.Context, workspaceID string) (Inputs, error) {
	sub, found, err := l.Subscriptions.Get(ctx, workspaceID)
	if err != nil {
		return Inputs{}, fmt.Errorf("read subscription: %w", err)
	}
	p, err := l.planOf(ctx, sub, found)
	if err != nil {
		return Inputs{}, err
	}
	status, err := l.Status.Status(ctx, workspaceID)
	if err != nil {
		return Inputs{}, fmt.Errorf("read workspace status: %w", err)
	}
	in := Inputs{Plan: p, WorkspaceStatus: status}
	if found {
		in.Version, in.Overrides = sub.Version, sub.LimitOverrides
	}
	if err := l.addTenant(ctx, workspaceID, &in); err != nil {
		return Inputs{}, err
	}
	return in, nil
}

// planOf returns the plan of a subscription row, or the free plan without one.
func (l Loader) planOf(ctx context.Context, sub subscription.Subscription, found bool) (plan.Plan, error) {
	key := plan.FreeKey
	if found {
		key = sub.PlanKey
	}
	p, ok, err := l.Plans.Get(ctx, key)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("read plan %q: %w", key, err)
	}
	if !ok {
		return plan.Plan{}, fmt.Errorf("plan %q does not exist", key)
	}
	return p, nil
}

// addTenant sets the tenant fields of in: the largest max_workspaces among the
// workspaces of the tenant, read for every one of them so that all of them get
// the same value (spec R6 section 5.3). A workspace with no tenant link keeps the
// gate off.
func (l Loader) addTenant(ctx context.Context, workspaceID string, in *Inputs) error {
	if l.Tenants == nil {
		return nil
	}
	ids, err := l.Tenants.SiblingIDs(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("read tenant workspaces: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	siblings := make([]Sibling, 0, len(ids))
	for _, id := range ids {
		if id == workspaceID {
			siblings = append(siblings, Sibling{Plan: in.Plan, Overrides: in.Overrides})
			continue
		}
		sub, found, err := l.Subscriptions.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("read subscription of a tenant workspace: %w", err)
		}
		p, err := l.planOf(ctx, sub, found)
		if err != nil {
			return fmt.Errorf("tenant workspace %s: %w", id, err)
		}
		s := Sibling{Plan: p}
		if found {
			s.Overrides = sub.LimitOverrides
		}
		siblings = append(siblings, s)
	}
	in.HasTenant = true
	in.MaxWorkspaces = TenantMaxWorkspaces(siblings)
	return nil
}
