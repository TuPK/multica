package workspaceexport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// DBStore is Store over the workspace_export table.
type DBStore struct {
	q  *db.Queries
	tx Beginner
}

// NewDBStore returns the database queue. tx starts the transaction in which an
// export's finish and its audit row commit together.
func NewDBStore(q *db.Queries, tx Beginner) *DBStore { return &DBStore{q: q, tx: tx} }

// auditActorLabel is the actor_label of the audit rows the job writes: the job
// has no requesting user, so the actor is the system.
const auditActorLabel = "system: workspace_export"

// finish runs change, which moves a running export to its final status, and
// writes the audit row of entry in the same transaction. It reports false, and
// writes nothing, when change finds no running export.
func (s *DBStore) finish(ctx context.Context, change func(q *db.Queries) (db.WorkspaceExport, error), entry func(db.WorkspaceExport) audit.Entry) (bool, error) {
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	row, err := change(qtx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := audit.Record(ctx, auditstore.New(qtx), entry(row)); err != nil {
		return false, fmt.Errorf("audit export %s: %w", util.UUIDToString(row.ID), err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// exportEntry is the audit entry of an export that finished. The detail names
// the requester and, for a finished archive, its size. It never carries the
// storage key or the error text, which can hold a path or a provider message.
func exportEntry(action string, row db.WorkspaceExport, detail map[string]any) audit.Entry {
	detail["requested_by"] = util.UUIDToString(row.RequestedBy)
	return audit.Entry{
		WorkspaceID: util.UUIDToString(row.WorkspaceID),
		ActorType:   audit.ActorSystem,
		ActorLabel:  auditActorLabel,
		Action:      action,
		TargetType:  "workspace_export",
		TargetID:    util.UUIDToString(row.ID),
		Detail:      detail,
	}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func optTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// FromRow converts a database row.
func FromRow(r db.WorkspaceExport) Export {
	return Export{
		ID:          util.UUIDToString(r.ID),
		WorkspaceID: util.UUIDToString(r.WorkspaceID),
		RequestedBy: util.UUIDToString(r.RequestedBy),
		Status:      r.Status,
		StorageKey:  r.StorageKey,
		SizeBytes:   r.SizeBytes,
		Error:       r.Error,
		CreatedAt:   r.CreatedAt.Time,
		StartedAt:   optTime(r.StartedAt),
		FinishedAt:  optTime(r.FinishedAt),
		ExpiresAt:   optTime(r.ExpiresAt),
	}
}

func (s *DBStore) ClaimNext(ctx context.Context, now time.Time) (Export, bool, error) {
	row, err := s.q.ClaimNextWorkspaceExport(ctx, ts(now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Export{}, false, nil
	}
	if err != nil {
		return Export{}, false, err
	}
	return FromRow(row), true, nil
}

func (s *DBStore) MarkDone(ctx context.Context, id string, finishedAt, expiresAt time.Time, storageKey string, sizeBytes int64) (bool, error) {
	uid, err := util.ParseUUID(id)
	if err != nil {
		return false, fmt.Errorf("parse export id: %w", err)
	}
	return s.finish(ctx,
		func(q *db.Queries) (db.WorkspaceExport, error) {
			return q.CompleteWorkspaceExport(ctx, db.CompleteWorkspaceExportParams{
				ID: uid, StorageKey: storageKey, SizeBytes: sizeBytes, FinishedAt: ts(finishedAt), ExpiresAt: ts(expiresAt),
			})
		},
		func(row db.WorkspaceExport) audit.Entry {
			return exportEntry(audit.ActionWorkspaceExportCompleted, row, map[string]any{"size_bytes": row.SizeBytes})
		})
}

func (s *DBStore) MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) (bool, error) {
	uid, err := util.ParseUUID(id)
	if err != nil {
		return false, fmt.Errorf("parse export id: %w", err)
	}
	return s.finish(ctx,
		func(q *db.Queries) (db.WorkspaceExport, error) {
			return q.FailWorkspaceExport(ctx, db.FailWorkspaceExportParams{ID: uid, Error: message, FinishedAt: ts(finishedAt)})
		},
		func(row db.WorkspaceExport) audit.Entry {
			return exportEntry(audit.ActionWorkspaceExportFailed, row, map[string]any{})
		})
}

func (s *DBStore) FailStale(ctx context.Context, startedBefore, now time.Time, message string) (int, error) {
	n, err := s.q.FailStaleWorkspaceExports(ctx, db.FailStaleWorkspaceExportsParams{
		Error: message, FinishedAt: ts(now), StartedBefore: ts(startedBefore),
	})
	return int(n), err
}

func (s *DBStore) ListExpired(ctx context.Context, now time.Time, limit int) ([]Export, error) {
	rows, err := s.q.ListExpiredWorkspaceExports(ctx, db.ListExpiredWorkspaceExportsParams{Now: ts(now), RowLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Export, len(rows))
	for i, r := range rows {
		out[i] = FromRow(r)
	}
	return out, nil
}

func (s *DBStore) MarkExpired(ctx context.Context, id string) (bool, error) {
	uid, err := util.ParseUUID(id)
	if err != nil {
		return false, fmt.Errorf("parse export id: %w", err)
	}
	n, err := s.q.ExpireWorkspaceExport(ctx, uid)
	return n == 1, err
}
