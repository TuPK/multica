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
}

// Inputs implements Source. A workspace with no subscription row is on the free
// plan at version 0. A subscription on a plan that no longer exists is an
// error: the caller answers 5xx, which Multica reads as "platform unavailable",
// rather than guessing a plan.
func (l Loader) Inputs(ctx context.Context, workspaceID string) (Inputs, error) {
	sub, found, err := l.Subscriptions.Get(ctx, workspaceID)
	if err != nil {
		return Inputs{}, fmt.Errorf("read subscription: %w", err)
	}
	key := plan.FreeKey
	if found {
		key = sub.PlanKey
	}
	p, ok, err := l.Plans.Get(ctx, key)
	if err != nil {
		return Inputs{}, fmt.Errorf("read plan %q: %w", key, err)
	}
	if !ok {
		return Inputs{}, fmt.Errorf("plan %q does not exist", key)
	}
	status, err := l.Status.Status(ctx, workspaceID)
	if err != nil {
		return Inputs{}, fmt.Errorf("read workspace status: %w", err)
	}
	in := Inputs{Plan: p, WorkspaceStatus: status}
	if found {
		in.Version, in.Overrides = sub.Version, sub.LimitOverrides
	}
	return in, nil
}
