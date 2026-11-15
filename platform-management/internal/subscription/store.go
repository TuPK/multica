package subscription

import (
	"context"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Tx is the reads and writes of one subscription change, all inside one
// transaction with its audit row.
type Tx interface {
	// WorkspaceName returns found == false for an unknown workspace.
	WorkspaceName(ctx context.Context, id string) (name string, found bool, err error)
	// Get reads the workspace's row and locks it. A missing row locks nothing.
	Get(ctx context.Context, workspaceID string) (s Subscription, found bool, err error)
	// Insert writes the first row, at version 1. It reports false when another
	// transaction wrote the row first.
	Insert(ctx context.Context, s Subscription) (written Subscription, ok bool, err error)
	// Update replaces the changeable fields and moves the version up by one.
	Update(ctx context.Context, s Subscription) (Subscription, error)
	// Plan reads a plan.
	Plan(ctx context.Context, key string) (p plan.Plan, found bool, err error)

	audit.Writer
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}
