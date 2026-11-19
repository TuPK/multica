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

-- name: GetDeletionAccount :one
-- The account a deletion request targets: its email, its platform status
-- (active when it has no row) and whether it is an active staff member.
SELECT u.id, u.email, COALESCE(pa.status, 'active')::text AS status,
       EXISTS (SELECT 1 FROM platform_staff s WHERE s.user_id = u.id AND s.revoked_at IS NULL) AS is_staff
FROM "user" u
LEFT JOIN platform_account pa ON pa.user_id = u.id
WHERE u.id = $1;

-- name: ListDeletionAccountWorkspaces :many
-- The workspaces an account belongs to, with the owner and member counts that
-- decide the sole-owner rule (spec R6 section 7.3).
SELECT m.workspace_id, w.slug, m.role,
       (SELECT count(*) FROM member o WHERE o.workspace_id = m.workspace_id AND o.role = 'owner')::bigint AS owner_count,
       (SELECT count(*) FROM member a WHERE a.workspace_id = m.workspace_id)::bigint AS member_count
FROM member m
JOIN workspace w ON w.id = m.workspace_id
WHERE m.user_id = $1
ORDER BY m.workspace_id;

-- name: GetDeletionMember :one
-- A member of a workspace with the member's account status, for the successor
-- check of spec GĐ11. No row when the user is not a member.
SELECT m.role, COALESCE(pa.status, 'active')::text AS status
FROM member m
LEFT JOIN platform_account pa ON pa.user_id = m.user_id
WHERE m.workspace_id = $1 AND m.user_id = $2;

-- name: ScheduleAccountDeletionNow :exec
-- Phase 1 of an account deletion (spec R6 section 7.4): the account is pending
-- deletion with the date now, and every session issued before now stops working.
INSERT INTO platform_account (user_id, status, reason, changed_by, sessions_valid_after, deletion_scheduled_at)
VALUES (sqlc.arg(user_id), 'pending_deletion', sqlc.arg(reason), sqlc.arg(changed_by), sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz)
ON CONFLICT (user_id) DO UPDATE
SET status = 'pending_deletion',
    reason = EXCLUDED.reason,
    changed_by = EXCLUDED.changed_by,
    changed_at = now(),
    sessions_valid_after = EXCLUDED.sessions_valid_after,
    deletion_scheduled_at = EXCLUDED.deletion_scheduled_at;

-- name: GetDeletionRuntime :one
-- The runtime a runtime deletion request targets: its name, workspace and tenant
-- (empty when the workspace has none). No row when the runtime is unknown.
SELECT r.id, r.name, r.workspace_id, COALESCE(tw.tenant_id::text, '')::text AS tenant_id
FROM agent_runtime r
LEFT JOIN tenant_workspace tw ON tw.workspace_id = r.workspace_id
WHERE r.id = $1;

-- name: AwaitDeletionApproval :one
-- Holds a pending request for a second system_admin (spec R6 section 3c.6, rule 2).
UPDATE platform_deletion_request
SET state = 'awaiting_approval', updated_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING *;

-- name: ApproveDeletionRequest :one
-- Moves an awaiting request to pending, with its approver (rule 3).
UPDATE platform_deletion_request
SET state = 'pending', approved_by = $2, approved_at = now(), updated_at = now()
WHERE id = $1 AND state = 'awaiting_approval'
RETURNING *;

-- name: RevertDeletionApproval :one
-- Puts an approved request back to awaiting when its phase 1 fails.
UPDATE platform_deletion_request
SET state = 'awaiting_approval', approved_by = NULL, approved_at = NULL, updated_at = now()
WHERE id = $1 AND state = 'pending' AND approved_by IS NOT NULL
RETURNING *;

-- name: RejectDeletionApproval :one
UPDATE platform_deletion_request
SET state = 'canceled', error_code = 'rejected', updated_at = now(), completed_at = now()
WHERE id = $1 AND state = 'awaiting_approval'
RETURNING *;

-- name: ExpireDeletionApprovals :many
-- Cancels the awaiting requests older than the cutoff (rule 4).
UPDATE platform_deletion_request
SET state = 'canceled', error_code = 'approval_expired', updated_at = now(), completed_at = now()
WHERE state = 'awaiting_approval' AND created_at < $1
RETURNING *;

-- name: CountActiveSystemAdmins :one
-- Active system administrators (rule 5): an approval needs one besides the requester.
SELECT count(*)::bigint FROM platform_staff
WHERE role = 'system_admin' AND revoked_at IS NULL;
