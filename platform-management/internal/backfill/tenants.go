package backfill

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// TenantTx is one batch's transaction of the tenant backfill.
type TenantTx interface {
	// WithoutTenant returns up to limit workspaces that have no tenant link, in id
	// order.
	WithoutTenant(ctx context.Context, limit int) ([]Candidate, error)
	// InsertTenant inserts the tenant keyed by the workspace id and named after
	// it. It returns false when the tenant already exists.
	InsertTenant(ctx context.Context, workspaceID string) (bool, error)
	// LinkWorkspace links the workspace to the tenant keyed by its own id. It
	// returns false when the workspace is already linked.
	LinkWorkspace(ctx context.Context, workspaceID string) (bool, error)
}

// TenantRunner opens one transaction per batch.
type TenantRunner interface {
	InTx(ctx context.Context, fn func(TenantTx) error) error
}

// TenantResult counts what one RunTenants did.
type TenantResult struct {
	Batches int
	Tenants int
	Linked  int
}

// RunTenants gives every workspace with no tenant link a tenant of its own, one
// batch per transaction (spec R6 section 5.4, step 5). It repairs the workspaces
// an old server created after migration 1021 copied the data. It stops at the
// first error; the batch that failed is rolled back and the batches before it
// stay committed, so running it again continues where it stopped.
func RunTenants(ctx context.Context, runner TenantRunner, batchSize int, log *slog.Logger) (TenantResult, error) {
	var res TenantResult
	if batchSize < 1 {
		return res, errors.New("backfill: batch size must be at least 1")
	}
	for {
		var fetched, tenants, linked int
		err := runner.InTx(ctx, func(tx TenantTx) error {
			fetched, tenants, linked = 0, 0, 0
			batch, err := tx.WithoutTenant(ctx, batchSize)
			if err != nil {
				return err
			}
			fetched = len(batch)
			for _, w := range batch {
				// The two inserts ignore a conflict, so a run that overlaps another
				// one, or a workspace linked since the batch was read, is not an error.
				made, err := tx.InsertTenant(ctx, w.ID)
				if err != nil {
					return err
				}
				joined, err := tx.LinkWorkspace(ctx, w.ID)
				if err != nil {
					return err
				}
				if made {
					tenants++
				}
				if joined {
					linked++
				}
			}
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("backfill tenants batch %d: %w", res.Batches+1, err)
		}
		if fetched == 0 {
			return res, nil
		}
		res.Batches++
		res.Tenants += tenants
		res.Linked += linked
		log.Info("tenant backfill batch committed", "batch", res.Batches, "fetched", fetched, "tenants", tenants, "linked", linked)
	}
}
