-- name: WorkspaceState :one
-- The platform writes platform_workspace, except for the owner's schedule and
-- cancel of a deletion below. A missing row means the workspace is active.
SELECT status, deletion_scheduled_at
FROM platform_workspace
WHERE workspace_id = $1;

-- name: ScheduleWorkspaceDeletion :one
-- Marks the workspace pending deletion. A workspace that is already pending
-- keeps its schedule and returns no row, so a repeated request cannot push the
-- date out.
INSERT INTO platform_workspace (workspace_id, status, reason, changed_by, changed_at, deletion_scheduled_at)
VALUES (@workspace_id, 'pending_deletion', @reason, @changed_by, now(), @scheduled_at)
ON CONFLICT (workspace_id) DO UPDATE
SET status = 'pending_deletion',
    reason = EXCLUDED.reason,
    changed_by = EXCLUDED.changed_by,
    changed_at = now(),
    deletion_scheduled_at = EXCLUDED.deletion_scheduled_at
WHERE platform_workspace.status <> 'pending_deletion'
RETURNING deletion_scheduled_at;

-- name: CancelWorkspaceDeletion :one
-- Returns the workspace to active only while it is pending deletion.
UPDATE platform_workspace
SET status = 'active',
    reason = '',
    changed_by = @changed_by,
    changed_at = now(),
    deletion_scheduled_at = NULL
WHERE workspace_id = @workspace_id
  AND status = 'pending_deletion'
RETURNING workspace_id;

-- name: ListDueWorkspaceDeletions :many
SELECT workspace_id
FROM platform_workspace
WHERE status = 'pending_deletion'
  AND deletion_scheduled_at <= @now
ORDER BY deletion_scheduled_at
LIMIT @row_limit;

-- name: LockPendingWorkspaceDeletion :one
-- Locks the row of a workspace that is still pending deletion. A cancel or a
-- second run waits on the lock, then finds the row changed or gone.
SELECT deletion_scheduled_at
FROM platform_workspace
WHERE workspace_id = $1
  AND status = 'pending_deletion'
FOR UPDATE;

-- name: DeletePlatformWorkspace :exec
DELETE FROM platform_workspace
WHERE workspace_id = $1;
