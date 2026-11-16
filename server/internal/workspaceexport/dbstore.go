package workspaceexport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// DBStore is Store over the workspace_export table.
type DBStore struct{ q *db.Queries }

// NewDBStore returns the database queue.
func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

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
	n, err := s.q.CompleteWorkspaceExport(ctx, db.CompleteWorkspaceExportParams{
		ID: uid, StorageKey: storageKey, SizeBytes: sizeBytes, FinishedAt: ts(finishedAt), ExpiresAt: ts(expiresAt),
	})
	return n == 1, err
}

func (s *DBStore) MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) (bool, error) {
	uid, err := util.ParseUUID(id)
	if err != nil {
		return false, fmt.Errorf("parse export id: %w", err)
	}
	n, err := s.q.FailWorkspaceExport(ctx, db.FailWorkspaceExportParams{ID: uid, Error: message, FinishedAt: ts(finishedAt)})
	return n == 1, err
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
