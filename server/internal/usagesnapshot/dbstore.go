package usagesnapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// DBStore is Store over the Multica database.
type DBStore struct {
	pool Beginner
	q    *db.Queries
}

// NewDBStore returns the database store.
func NewDBStore(pool Beginner) *DBStore { return &DBStore{pool: pool, q: db.New(pool)} }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// WorkspaceIDs implements Store. An empty after starts from the first id.
func (s *DBStore) WorkspaceIDs(ctx context.Context, after string, limit int) ([]string, error) {
	start := pgtype.UUID{Valid: true} // all zero bytes sorts before every id
	if after != "" {
		u, err := util.ParseUUID(after)
		if err != nil {
			return nil, err
		}
		start = u
	}
	ids, err := s.q.ListUsageWorkspaceIDs(ctx, db.ListUsageWorkspaceIDsParams{AfterID: start, RowLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = util.UUIDToString(id)
	}
	return out, nil
}

// Counts implements Store by running the query of each Metric.
func (s *DBStore) Counts(ctx context.Context, workspaceID string, w Window) (Counts, error) {
	ws, err := util.ParseUUID(workspaceID)
	if err != nil {
		return Counts{}, err
	}
	start, end := ts(w.Start), ts(w.End)
	var c Counts
	steps := []struct {
		column string
		run    func() error
	}{
		{"members", func() (err error) {
			c.Members, err = s.q.CountUsageMembers(ctx, db.CountUsageMembersParams{WorkspaceID: ws, DayEnd: end})
			return
		}},
		{"agents", func() (err error) {
			c.Agents, err = s.q.CountUsageAgents(ctx, db.CountUsageAgentsParams{WorkspaceID: ws, DayEnd: end})
			return
		}},
		{"runtimes", func() (err error) {
			c.Runtimes, err = s.q.CountUsageRuntimes(ctx, db.CountUsageRuntimesParams{WorkspaceID: ws, DayEnd: end})
			return
		}},
		{"issues_total", func() (err error) {
			c.IssuesTotal, err = s.q.CountUsageIssuesTotal(ctx, db.CountUsageIssuesTotalParams{WorkspaceID: ws, DayEnd: end})
			return
		}},
		{"issues_created", func() (err error) {
			c.IssuesCreated, err = s.q.CountUsageIssuesCreated(ctx, db.CountUsageIssuesCreatedParams{WorkspaceID: ws, DayStart: start, DayEnd: end})
			return
		}},
		{"tasks_run", func() (err error) {
			c.TasksRun, err = s.q.CountUsageTasksRun(ctx, db.CountUsageTasksRunParams{WorkspaceID: ws, DayStart: start, DayEnd: end})
			return
		}},
		{"storage_bytes", func() (err error) {
			c.StorageBytes, err = s.q.SumUsageStorageBytes(ctx, db.SumUsageStorageBytesParams{WorkspaceID: ws, DayEnd: end})
			return
		}},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			return Counts{}, fmt.Errorf("count %s: %w", step.column, err)
		}
	}
	return c, nil
}

// Upsert implements Store: all rows in one transaction.
func (s *DBStore) Upsert(ctx context.Context, rows []Row) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	q := s.q.WithTx(tx)
	for _, r := range rows {
		ws, err := util.ParseUUID(r.WorkspaceID)
		if err != nil {
			return err
		}
		if err := q.UpsertPlatformUsageDaily(ctx, db.UpsertPlatformUsageDailyParams{
			WorkspaceID: ws, Day: pgtype.Date{Time: r.Day, Valid: true},
			Members: r.Members, Agents: r.Agents, Runtimes: r.Runtimes, IssuesTotal: r.IssuesTotal,
			IssuesCreated: r.IssuesCreated, TasksRun: r.TasksRun, StorageBytes: r.StorageBytes,
		}); err != nil {
			return fmt.Errorf("upsert %s: %w", r.WorkspaceID, err)
		}
	}
	return tx.Commit(ctx)
}
