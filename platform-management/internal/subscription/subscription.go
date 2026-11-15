// Package subscription owns a workspace's subscription: the stored row, the
// staff write that changes it, and the two Cloud routes Multica's billing page
// reads (summary and prices).
package subscription

import (
	"context"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Statuses. A new row is active; later steps (renewal, grace, cancellation)
// move it.
const StatusActive = "active"

// Billing intervals.
const (
	IntervalMonth = "month"
	IntervalYear  = "year"
)

// Subscription is one row of workspace_subscription. A workspace with no row is
// on the free plan, with Version 0.
type Subscription struct {
	WorkspaceID        string
	PlanKey            string
	Status             string
	Seats              int
	Interval           string
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	TrialEnd           *time.Time
	GraceUntil         *time.Time
	CancelAtPeriodEnd  bool
	LimitOverrides     plan.Limits
	Version            int64
	UpdatedAt          time.Time
}

// Reader reads a workspace's subscription row.
type Reader interface {
	// Get returns found == false for a workspace with no row.
	Get(ctx context.Context, workspaceID string) (s Subscription, found bool, err error)
}

// Invalidator drops the cached entitlement policy after a write.
// *entitlement.Service is one.
type Invalidator interface {
	// Invalidate drops one workspace's entry.
	Invalidate(workspaceID string)
	// InvalidateAll drops every entry, for a change to a plan.
	InvalidateAll()
}
