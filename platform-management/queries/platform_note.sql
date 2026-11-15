-- name: ListPlatformNotes :many
-- Newest first, keyset-paged on (created_at, id). The author's email is NULL
-- when the account no longer exists.
SELECT n.id, n.workspace_id, n.author_id, u.email AS author_email, n.body, n.created_at
FROM platform_note n
LEFT JOIN "user" u ON u.id = n.author_id
WHERE n.workspace_id = $1
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (n.created_at, n.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY n.created_at DESC, n.id DESC
LIMIT sqlc.arg('row_limit');

-- name: InsertPlatformNote :one
INSERT INTO platform_note (workspace_id, author_id, body)
VALUES ($1, $2, $3)
RETURNING id, created_at;
