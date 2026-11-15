-- name: InsertAuditEvent :exec
INSERT INTO audit_event (
    workspace_id, actor_type, actor_id, actor_label, action,
    target_type, target_id, target_label, detail, ip, user_agent, request_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
);

-- name: ListWorkspaceAuditEvents :many
-- Newest first, keyset-paged on (created_at, id). from is inclusive, to is
-- exclusive. Callers ask for one row more than they return to learn whether
-- another page exists.
SELECT * FROM audit_event
WHERE workspace_id = $1
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('actor_id')::text IS NULL OR actor_id = sqlc.narg('actor_id')::text)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR created_at < sqlc.narg('created_to')::timestamptz)
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: ListUserSecurityEvents :many
-- Account-level events (no workspace) performed by one user.
SELECT * FROM audit_event
WHERE workspace_id IS NULL
  AND actor_id = sqlc.arg('actor_id')::text
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: DeleteAuditEventsBefore :execrows
-- Retention. Deletes at most row_limit rows older than the cutoff.
DELETE FROM audit_event
WHERE id IN (
    SELECT old.id FROM audit_event AS old WHERE old.created_at < $1 LIMIT $2
);
