-- name: GetPlatformWorkspace :one
SELECT * FROM platform_workspace
WHERE workspace_id = $1;

-- name: GetPlatformWorkspaceForUpdate :one
-- Locks the row for the rest of the transaction. A missing row locks nothing;
-- the unique index on workspace_id settles two concurrent first writes.
SELECT * FROM platform_workspace
WHERE workspace_id = $1
FOR UPDATE;

-- name: UpsertPlatformWorkspaceStatus :execrows
-- Writes only when the status changes, and returns the rows written (0 or 1).
INSERT INTO platform_workspace (workspace_id, status, reason, changed_by, deletion_scheduled_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id) DO UPDATE
SET status = EXCLUDED.status,
    reason = EXCLUDED.reason,
    changed_by = EXCLUDED.changed_by,
    changed_at = now(),
    deletion_scheduled_at = EXCLUDED.deletion_scheduled_at
WHERE platform_workspace.status IS DISTINCT FROM EXCLUDED.status;

-- name: ListNonActivePlatformWorkspaces :many
SELECT * FROM platform_workspace
WHERE status <> 'active'
ORDER BY changed_at DESC, workspace_id;

-- name: InsertAuditEvent :exec
INSERT INTO audit_event (
    workspace_id, actor_type, actor_id, actor_label, action,
    target_type, target_id, target_label, detail, ip, user_agent, request_id,
    tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
);
