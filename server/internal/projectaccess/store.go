package projectaccess

import (
	"context"

	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TxStarter begins a transaction.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// inTx runs fn with queries bound to one transaction and commits when fn
// succeeds. Any error, including the audit write, rolls everything back.
func inTx(ctx context.Context, starter TxStarter, q *db.Queries, fn func(qtx *db.Queries, tx pgx.Tx) error) error {
	tx, err := starter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after Commit
	if err := fn(q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
