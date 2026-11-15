package plan

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Tx is the plan writes of one operation, all inside one transaction with its
// audit row.
type Tx interface {
	Count(ctx context.Context) (int64, error)
	// InsertIfAbsent adds a plan and reports whether it did: false when a plan
	// with the key exists.
	InsertIfAbsent(ctx context.Context, p Plan) (bool, error)
	// Get reads a plan and locks its row.
	Get(ctx context.Context, key string) (p Plan, found bool, err error)
	// Upsert creates the plan or replaces every field of it.
	Upsert(ctx context.Context, p Plan) error
	// Delete removes a plan and reports whether there was one.
	Delete(ctx context.Context, key string) (bool, error)
	// CountSubscriptions counts the subscription rows that name the plan.
	CountSubscriptions(ctx context.Context, key string) (int64, error)

	audit.Writer
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Seed loads the embedded catalogue when the plan table is empty, and does
// nothing otherwise: once the table has rows, staff own it. It returns the
// number of plans it inserted. The load is one transaction, so a failure leaves
// the table empty and the next start tries again.
func Seed(ctx context.Context, r Runner, log *slog.Logger) (int, error) {
	catalogue, err := Catalogue()
	if err != nil {
		return 0, err
	}
	for _, p := range catalogue {
		if err := Validate(p); err != nil {
			return 0, fmt.Errorf("plan catalogue: %s: %w", p.Key, err)
		}
	}
	inserted := 0
	err = r.InTx(ctx, func(tx Tx) error {
		inserted = 0
		n, err := tx.Count(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		for _, p := range catalogue {
			ok, err := tx.InsertIfAbsent(ctx, p)
			if err != nil {
				return fmt.Errorf("insert plan %s: %w", p.Key, err)
			}
			if ok {
				inserted++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if inserted > 0 {
		log.Info("plan catalogue loaded", "plans", inserted)
	}
	return inserted, nil
}
