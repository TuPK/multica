-- Staff deletion requests (spec R6 section 6). The platform writes the request
-- and cancels or retries it; Multica's staff_deletion job moves it on and writes
-- state, error_code, attempts, target_label, updated_at and completed_at.

-- name: InsertDeletionRequest :one
-- Inserts a pending request. A live request for the same target makes the
-- insert return no row (partial unique index of migration 1027); the caller
-- then reads the live one.
INSERT INTO platform_deletion_request (
    id, kind, target_id, target_label, reason, state, requested_by, requested_by_label, export_first, resolutions, parent_id, tenant_id
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $10
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
-- The newest requests first, for system_admin. kind and state filter when non-empty.
SELECT * FROM platform_deletion_request
WHERE ($1::text = '' OR kind = $1)
  AND ($2::text = '' OR state = $2)
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: ListDeletionRequestsInTenant :many
-- The newest workspace and tenant requests of one tenant, for its admin. Account
-- requests carry no tenant_id and never match.
SELECT * FROM platform_deletion_request
WHERE tenant_id = $1
  AND kind IN ('workspace', 'tenant')
  AND ($2::text = '' OR kind = $2)
  AND ($3::text = '' OR state = $3)
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: GetDeletionWorkspace :one
-- The workspace a deletion request targets: its name, slug, platform status
-- (active when it has no row) and its tenant (null when it has none).
SELECT w.id, w.name, w.slug, COALESCE(pw.status, 'active')::text AS status, tw.tenant_id
FROM workspace w
LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
LEFT JOIN tenant_workspace tw ON tw.workspace_id = w.id
WHERE w.id = $1;

-- name: GetDeletionTenant :one
SELECT id, name, status FROM tenant WHERE id = $1;

-- name: ListDeletionTenantWorkspaces :many
-- The workspaces of a tenant, in a fixed order. The caller takes LockDeletionTenant
-- first, so the list is final for the transaction.
SELECT workspace_id FROM tenant_workspace WHERE tenant_id = $1 ORDER BY workspace_id;

-- name: LockDeletionTenant :exec
-- The max_workspaces gate lock of the tenant (server plan_limit.sql LockPlanGate),
-- so a workspace creation and this request queue on the same lock.
SELECT pg_advisory_xact_lock(hashtext($1::text), hashtext('max_workspaces'));
