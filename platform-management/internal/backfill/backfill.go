// Package backfill gives every workspace that has no subscription row the free
// plan. It is the cutover step of gate G-R2: run once per environment, and safe
// to run again, because a workspace that has a row is never touched.
package backfill

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/multica-ai/multica/platform-management/audit"
)

const (
	// BatchSize is the number of workspaces per transaction.
	BatchSize = 500

	// Plan is the transitional plan: the free plan's limits apply, and data over
	// them stays (spec 5.9).
	Plan = "free"
	// Source marks the audit rows this step wrote.
	Source = "cutover"
	// ActorLabel names the command in the audit log.
	ActorLabel = "platform-backfill"
)

// Candidate is a workspace with no subscription row.
type Candidate struct {
	ID   string
	Name string
}

// Tx is one batch's transaction. Its audit writes commit or roll back with its
// inserts.
type Tx interface {
	// Unsubscribed returns up to limit workspaces with no subscription row, in id order.
	Unsubscribed(ctx context.Context, limit int) ([]Candidate, error)
	// InsertFree inserts the free row. It returns false when a row already
	// exists, which another writer may have made since the batch was read.
	InsertFree(ctx context.Context, workspaceID string) (bool, error)
	audit.Writer
}

// Runner opens one transaction per batch.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Result counts what one Run did.
type Result struct {
	Batches  int
	Inserted int
}

// Run inserts the free row for every workspace with no subscription row, one
// batch per transaction. It stops at the first error; the batch that failed is
// rolled back, and the batches before it stay committed.
func Run(ctx context.Context, runner Runner, batchSize int, log *slog.Logger) (Result, error) {
	var res Result
	if batchSize < 1 {
		return res, errors.New("backfill: batch size must be at least 1")
	}
	for {
		var fetched, inserted int
		err := runner.InTx(ctx, func(tx Tx) error {
			fetched, inserted = 0, 0
			batch, err := tx.Unsubscribed(ctx, batchSize)
			if err != nil {
				return err
			}
			fetched = len(batch)
			for _, w := range batch {
				ok, err := tx.InsertFree(ctx, w.ID)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if err := audit.Record(ctx, tx, audit.Entry{
					WorkspaceID: w.ID,
					ActorType:   audit.ActorSystem,
					ActorLabel:  ActorLabel,
					Action:      audit.ActionSubscriptionBackfilled,
					TargetType:  "workspace",
					TargetID:    w.ID,
					TargetLabel: w.Name,
					// "plan", not "plan_key": audit.Record redacts keys containing "key".
					Detail: map[string]any{"plan": Plan, "source": Source},
				}); err != nil {
					return err
				}
				inserted++
			}
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("backfill batch %d: %w", res.Batches+1, err)
		}
		if fetched == 0 {
			return res, nil
		}
		res.Batches++
		res.Inserted += inserted
		log.Info("subscription backfill batch committed", "batch", res.Batches, "fetched", fetched, "inserted", inserted)
	}
}
