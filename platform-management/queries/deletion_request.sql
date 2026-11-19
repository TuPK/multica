-- Staff deletion requests (spec R6 section 6). The platform writes the request
-- and cancels or retries it; Multica's staff_deletion job moves it on and writes
-- state, error_code, attempts, target_label, updated_at and completed_at.

-- name: InsertDeletionRequest :one
-- Inserts a pending request. A live request for the same target makes the
-- insert return no row (partial unique index of migration 1027); the caller
-- then reads the live one.
INSERT INTO platform_deletion_request (
    id, kind, target_id, target_label, reason, state, requested_by, requested_by_label, export_first, resolutions, parent_id
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9
)
ON CONFLICT (kind, target_id) WHERE state IN ('awaiting_approval', 'pending', 'running', 'failed') DO NOTHING
RETURNING *;

-- name: GetDeletionRequest :one
SELECT * FROM platform_deletion_request
WHERE id = $1;

-- name: GetLiveDeletionRequest :one
SELECT * FROM platform_deletion_request
WHERE kind = $1 AND target_id = $2 AND state IN ('awaiting_approval', 'pending', 'running', 'failed');

-- name: LockDeletionRequest :one
SELECT * FROM platform_deletion_request
WHERE id = $1
FOR UPDATE;

-- name: CancelDeletionRequest :one
UPDATE platform_deletion_request
SET state = 'canceled', updated_at = now(), completed_at = now()
WHERE id = $1 AND state IN ('pending', 'failed')
RETURNING *;

-- name: RetryDeletionRequest :one
UPDATE platform_deletion_request
SET state = 'pending', error_code = '', attempts = 0, updated_at = now()
WHERE id = $1 AND state = 'failed'
RETURNING *;

-- name: ListDeletionRequests :many
-- The newest requests first. kind and state filter when non-empty; the
-- admin's tenant filter is applied by the caller through the target lists.
SELECT * FROM platform_deletion_request
WHERE ($1::text = '' OR kind = $1)
  AND ($2::text = '' OR state = $2)
ORDER BY created_at DESC, id DESC
LIMIT $3;
