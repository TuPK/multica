-- quota_request: an owner's request to raise one plan gate of their workspace
-- (spec R6 section 3c.5, D102). The platform owns the table, decides the requests
-- and writes limit_overrides; Multica writes a row when an owner asks, and reads
-- the workspace's own requests back.

-- name: InsertQuotaRequest :one
-- Files a pending request. A pending request on the same gate makes the insert
-- return no row (one pending per workspace and gate).
INSERT INTO quota_request (id, workspace_id, tenant_id, gate, current_value, requested_value,
                           state, reason, requested_by, requested_by_label, created_at)
VALUES (gen_random_uuid(), sqlc.arg('workspace_id'), sqlc.narg('tenant_id'), sqlc.arg('gate'),
        sqlc.narg('current_value'), sqlc.arg('requested_value'), 'pending', sqlc.arg('reason'),
        sqlc.narg('requested_by'), sqlc.arg('requested_by_label'), now())
ON CONFLICT (workspace_id, gate) WHERE state = 'pending' DO NOTHING
RETURNING *;

-- name: ListQuotaRequestsForWorkspace :many
SELECT * FROM quota_request
WHERE workspace_id = $1
ORDER BY created_at DESC, id DESC;

-- name: DeleteWorkspaceQuotaRequests :exec
-- The requests of a workspace go with it.
DELETE FROM quota_request WHERE workspace_id = $1;
