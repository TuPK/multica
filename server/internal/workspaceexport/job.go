package workspaceexport

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	// Batch is how many queued exports one run builds. The job runs every
	// minute, so the rest wait for the next run.
	Batch = 5
	// StaleAfter is how long an export may stay running before the job gives it
	// up as failed. Without it, a run that died would hold the workspace's one
	// export slot for good.
	StaleAfter = time.Hour

	expiryBatch   = 50
	maxErrorChars = 500
)

// Store is the queue of exports. Each method that changes a row applies only
// from the status the job expects, so a repeated call does nothing.
type Store interface {
	// ClaimNext moves the oldest queued export to running. It reports false
	// when none is queued.
	ClaimNext(ctx context.Context, now time.Time) (Export, bool, error)
	// MarkDone moves a running export to done; false means it was not running.
	MarkDone(ctx context.Context, id string, finishedAt, expiresAt time.Time, storageKey string, sizeBytes int64) (bool, error)
	// MarkFailed moves a running export to failed; false means it was not running.
	MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) (bool, error)
	// FailStale fails running exports started before startedBefore and returns
	// how many.
	FailStale(ctx context.Context, startedBefore, now time.Time, message string) (int, error)
	// ListExpired lists up to limit done exports whose expiry is at or before now.
	ListExpired(ctx context.Context, now time.Time, limit int) ([]Export, error)
	// MarkExpired moves a done export to expired; false means it was not done.
	MarkExpired(ctx context.Context, id string) (bool, error)
}

// Objects is the part of the storage package the job needs.
type Objects interface {
	Upload(ctx context.Context, key string, data []byte, contentType string, filename string) (string, error)
	DeleteObject(ctx context.Context, key string) error
}

// Runner is the export job: it builds queued exports and expires old ones.
type Runner struct {
	Store   Store
	Source  Source
	Objects Objects
	Now     func() time.Time
}

// StorageKey is where the archive of an export is stored.
func StorageKey(e Export) string {
	return fmt.Sprintf("workspace-exports/%s/%s.zip", e.WorkspaceID, e.ID)
}

// RunOnce fails stuck exports, builds up to Batch queued exports one at a time,
// then expires the done exports past their expiry. It returns how many exports
// it claimed. A failed build marks that export failed and is not retried; only
// a store error is returned.
func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	now := r.Now()
	if n, err := r.Store.FailStale(ctx, now.Add(-StaleAfter), now, "export timed out"); err != nil {
		return 0, err
	} else if n > 0 {
		slog.Warn("workspace export: failed exports that ran too long", "count", n)
	}

	claimed := 0
	for claimed < Batch && ctx.Err() == nil {
		e, ok, err := r.Store.ClaimNext(ctx, r.Now())
		if err != nil {
			return claimed, err
		}
		if !ok {
			break
		}
		claimed++
		if err := r.build(ctx, e); err != nil {
			return claimed, err
		}
	}

	return claimed, r.expire(ctx)
}

// build runs one claimed export. A failure to build or upload is recorded on
// the row; only a failure to record is returned.
func (r *Runner) build(ctx context.Context, e Export) error {
	key, size, err := r.buildAndUpload(ctx, e)
	if err != nil {
		slog.Warn("workspace export failed", "export_id", e.ID, "workspace_id", e.WorkspaceID, "error", err)
		_, markErr := r.Store.MarkFailed(ctx, e.ID, r.Now(), truncate(err.Error()))
		return markErr
	}
	finished := r.Now()
	ok, err := r.Store.MarkDone(ctx, e.ID, finished, ExpiresAt(finished), key, size)
	if err == nil && ok {
		return nil
	}
	// The row is no longer running (it was failed as stale) or could not be
	// updated: the object has no row that owns it.
	if delErr := r.Objects.DeleteObject(ctx, key); delErr != nil {
		slog.Warn("workspace export: delete of an unowned archive failed", "export_id", e.ID, "key", key, "error", delErr)
	}
	return err
}

func (r *Runner) buildAndUpload(ctx context.Context, e Export) (string, int64, error) {
	if r.Objects == nil {
		return "", 0, errors.New("file storage is not configured")
	}
	data, _, err := BuildArchive(ctx, r.Source, e.WorkspaceID, r.Now())
	if err != nil {
		return "", 0, err
	}
	key := StorageKey(e)
	if _, err := r.Objects.Upload(ctx, key, data, "application/zip", "workspace-"+e.ID+".zip"); err != nil {
		return "", 0, fmt.Errorf("upload archive: %w", err)
	}
	return key, int64(len(data)), nil
}

// expire deletes the stored archive of every done export past its expiry, then
// marks the row expired. A missing object is not an error. A failed delete
// leaves the row done, so the next run tries again.
func (r *Runner) expire(ctx context.Context) error {
	now := r.Now()
	due, err := r.Store.ListExpired(ctx, now, expiryBatch)
	if err != nil {
		return err
	}
	for _, e := range due {
		if e.StorageKey != "" {
			if err := r.Objects.DeleteObject(ctx, e.StorageKey); err != nil && !errors.Is(err, fs.ErrNotExist) {
				slog.Warn("workspace export: delete of an expired archive failed; the next run retries", "export_id", e.ID, "key", e.StorageKey, "error", err)
				continue
			}
		}
		if _, err := r.Store.MarkExpired(ctx, e.ID); err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string) string {
	if len(s) <= maxErrorChars {
		return s
	}
	return s[:maxErrorChars]
}

// NewRunner returns the job over the database and the object store.
func NewRunner(q *db.Queries, objects Objects) *Runner {
	return &Runner{Store: NewDBStore(q), Source: NewDBSource(q), Objects: objects, Now: time.Now}
}
