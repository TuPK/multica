package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// JobNameWorkspaceDeletion is the canonical name used in audit rows. Stable
// across releases.
const JobNameWorkspaceDeletion = "workspace_deletion"

// workspaceDeletionBatch bounds how many workspaces one run deletes. A
// teardown can take long on a large workspace, so the rest wait for the next
// hourly run.
const workspaceDeletionBatch = 20

// WorkspaceDeleter finds and carries out due workspace deletions. The handler
// package implements it, because the deletion reuses the teardown that
// DeleteWorkspace runs.
type WorkspaceDeleter interface {
	// DueWorkspaceDeletions lists up to limit workspace ids that are pending
	// deletion and due at now.
	DueWorkspaceDeletions(ctx context.Context, now time.Time, limit int) ([]string, error)
	// DeleteDueWorkspace deletes one workspace. It returns false, with no
	// error, when the workspace is no longer pending or not due.
	DeleteDueWorkspace(ctx context.Context, workspaceID string, now time.Time) (bool, error)
}

// WorkspaceStateInvalidator drops the cached workspace state of one workspace.
// The job calls it after a deletion, so no request reads a stale state for a
// workspace that is gone.
type WorkspaceStateInvalidator interface {
	Invalidate(ctx context.Context, workspaceID string) error
}

// WorkspaceDeletionJob runs hourly. Each run deletes at most
// workspaceDeletionBatch workspaces, each in its own transaction. A failure is
// logged and leaves that workspace pending for the next run, and a second run
// finds nothing left to do for a workspace that is already deleted.
func WorkspaceDeletionJob(deleter WorkspaceDeleter, state WorkspaceStateInvalidator) JobSpec {
	return JobSpec{
		Name:              JobNameWorkspaceDeletion,
		Cadence:           time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     time.Hour,
		RunTimeout:        30 * time.Minute,
		StaleTimeout:      35 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			deleted, err := runWorkspaceDeletions(ctx, deleter, state, time.Now())
			if err != nil {
				return HandlerResult{}, err
			}
			return HandlerResult{RowsAffected: int64(deleted)}, nil
		},
	}
}

// runWorkspaceDeletions processes one batch and returns how many workspaces it
// deleted. A failed workspace is logged and skipped, so one bad workspace
// cannot hold back the others. The cached state is dropped only for a workspace
// this run deleted.
func runWorkspaceDeletions(ctx context.Context, deleter WorkspaceDeleter, state WorkspaceStateInvalidator, now time.Time) (int, error) {
	ids, err := deleter.DueWorkspaceDeletions(ctx, now, workspaceDeletionBatch)
	if err != nil {
		return 0, err
	}
	deleted, failed := 0, 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		done, err := deleter.DeleteDueWorkspace(ctx, id, now)
		switch {
		case err != nil:
			failed++
			slog.Error("workspace deletion failed; the workspace stays pending and the next run retries", "workspace_id", id, "error", err)
		case done:
			deleted++
			if err := state.Invalidate(ctx, id); err != nil {
				slog.Warn("workspace deletion: state cache invalidation failed; stale for up to the cache TTL", "workspace_id", id, "error", err)
			}
		}
	}
	slog.Info("workspace deletion", "due", len(ids), "deleted", deleted, "failed", failed)
	return deleted, nil
}
