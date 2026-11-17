-- name: ListPendingAccountRequestIDs :many
SELECT id FROM platform_account_request
WHERE state = 'pending'
ORDER BY created_at, id
LIMIT $1;

-- name: LockPendingAccountRequest :one
-- SKIP LOCKED: no row means another run holds the request or it is no longer pending.
SELECT * FROM platform_account_request
WHERE id = $1 AND state = 'pending'
FOR UPDATE SKIP LOCKED;

-- name: MarkAccountRequestProvisioned :exec
UPDATE platform_account_request
SET state = 'provisioned', user_id = $2, workspace_id = $3, updated_at = now()
WHERE id = $1;

-- name: MarkAccountRequestFailed :exec
UPDATE platform_account_request
SET state = 'failed', error_code = $2, updated_at = now()
WHERE id = $1 AND state = 'pending';

-- name: IncrementAccountRequestAttempts :one
UPDATE platform_account_request
SET attempts = attempts + 1, updated_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING attempts;

-- name: ClearAccountRequestWorkspaceForWorkspace :exec
-- The table has no foreign key and the request row outlives the workspace.
UPDATE platform_account_request
SET workspace_id = NULL
WHERE workspace_id = $1;
