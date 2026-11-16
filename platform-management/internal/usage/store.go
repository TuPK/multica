package usage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

// Store reads the days to rate and writes their charge rows, over a Postgres
// pool.
type Store struct{ pool *pgxpool.Pool }

// NewStore returns a Store over pool.
func NewStore(pool *pgxpool.Pool) Store { return Store{pool: pool} }

// DaysToRate returns up to limit closed days (before today, UTC) that have a
// usage snapshot row and no charge rows, oldest first.
func (s Store) DaysToRate(ctx context.Context, today time.Time, limit int32) ([]db.ListUsageDaysToRateRow, error) {
	return db.New(s.pool).ListUsageDaysToRate(ctx, db.ListUsageDaysToRateParams{
		Today:    pgtype.Date{Time: today, Valid: true},
		RowLimit: limit,
	})
}

// WriteDay writes the rows of one workspace's day in one transaction, so a day
// is either fully rated or not at all. It returns how many rows were new: a row
// that already exists is left as it is.
func (s Store) WriteDay(ctx context.Context, workspaceID pgtype.UUID, day time.Time, rows []Row) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	var inserted int64
	for _, r := range rows {
		n, err := q.InsertUsageCharge(ctx, db.InsertUsageChargeParams{
			WorkspaceID: workspaceID,
			Day:         pgtype.Date{Time: day, Valid: true},
			Meter:       r.Meter,
			Quantity:    r.Quantity,
			PlanKey:     r.PlanKey,
			UnitPrice:   r.UnitPrice,
			Per:         r.Per,
			DiscountPct: int32(r.DiscountPct),
			NetAmount:   r.NetAmount,
			Note:        r.Note,
		})
		if err != nil {
			return 0, err
		}
		inserted += n
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return inserted, nil
}
