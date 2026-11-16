package usagesnapshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Store is what the job reads and writes. DBStore is the database
// implementation.
type Store interface {
	// WorkspaceIDs returns up to limit workspace ids greater than after, in
	// ascending order.
	WorkspaceIDs(ctx context.Context, after string, limit int) ([]string, error)
	// Counts computes one workspace's numbers for the window.
	Counts(ctx context.Context, workspaceID string, w Window) (Counts, error)
	// Upsert writes rows in one transaction. A row for a (workspace, day) that
	// exists replaces it.
	Upsert(ctx context.Context, rows []Row) error
}

// Job computes the usage rows of the day that just ended.
type Job struct {
	Store Store
	Log   *slog.Logger
	// BatchSize is the workspaces per transaction; it is capped at MaxBatch.
	BatchSize int
}

// New returns a Job over store with the largest batch.
func New(store Store) *Job {
	return &Job{Store: store, Log: slog.Default(), BatchSize: MaxBatch}
}

// Result says what a run did.
type Result struct {
	Day     time.Time
	Written int
	Skipped int
}

// Run computes the day DayFor(now) for every workspace and writes it, one
// transaction per batch. A workspace whose counts fail is logged and skipped,
// and a batch whose write fails is logged and skipped; the run goes on with the
// rest. It then returns an error if anything was skipped, so the scheduler
// retries the same day. A retry replaces the rows, so it is safe.
func (j *Job) Run(ctx context.Context, now time.Time) (Result, error) {
	day := DayFor(now)
	w := DayWindow(day)
	res := Result{Day: day}
	size := j.BatchSize
	if size <= 0 || size > MaxBatch {
		size = MaxBatch
	}
	log := j.Log
	if log == nil {
		log = slog.Default()
	}

	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		ids, err := j.Store.WorkspaceIDs(ctx, after, size)
		if err != nil {
			return res, fmt.Errorf("usage snapshot %s: list workspaces: %w", day.Format("2006-01-02"), err)
		}
		if len(ids) == 0 {
			break
		}
		after = ids[len(ids)-1]

		rows := make([]Row, 0, len(ids))
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			c, err := j.Store.Counts(ctx, id, w)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return res, err
				}
				log.Error("usage snapshot: count workspace", "workspace_id", id, "day", day.Format("2006-01-02"), "error", err)
				res.Skipped++
				continue
			}
			rows = append(rows, Row{WorkspaceID: id, Day: day, Counts: c})
		}
		if len(rows) == 0 {
			continue
		}
		if err := j.Store.Upsert(ctx, rows); err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return res, cerr
			}
			log.Error("usage snapshot: write batch", "rows", len(rows), "day", day.Format("2006-01-02"), "error", err)
			res.Skipped += len(rows)
			continue
		}
		res.Written += len(rows)
	}

	if res.Skipped > 0 {
		return res, fmt.Errorf("usage snapshot %s: %d of %d workspaces were skipped", day.Format("2006-01-02"), res.Skipped, res.Skipped+res.Written)
	}
	return res, nil
}

// RunUsageSnapshot is the scheduler's entry point (scheduler.UsageSnapshotRunner).
// It returns how many rows the run wrote.
func (j *Job) RunUsageSnapshot(ctx context.Context, runAt time.Time) (int, error) {
	res, err := j.Run(ctx, runAt)
	return res.Written, err
}
