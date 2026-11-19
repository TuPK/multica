-- Quota requests (spec R6 section 3c.5, QT7). The platform owns quota_request
-- (migrations 1035 to 1037); the request reads the subscription and the plan
-- catalogue for the effective limit.

-- name: InsertQuotaRequest :one
-- Inserts a pending request. A pending one for the same workspace and gate
-- makes the insert return no row; the caller answers 409.
INSERT INTO quota_request (
    id, workspace_id, tenant_id, gate, current_value, requested_value, reason,
    state, requested_by, requested_by_label
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, $5, $6, 'pending', $7, $8
)
ON CONFLICT (workspace_id, gate) WHERE state = 'pending' DO NOTHING
RETURNING *;

-- name: GetQuotaRequest :one
SELECT * FROM quota_request
WHERE id = $1;

-- name: LockQuotaRequest :one
SELECT * FROM quota_request
WHERE id = $1
FOR UPDATE;

-- name: DecideQuotaRequest :one
-- Moves a pending request to approved or denied. A closed request matches nothing.
UPDATE quota_request
SET state = $2, approved_value = $3, decision_reason = $4, decided_by = $5, decided_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING *;

-- name: ListQuotaRequests :many
-- The newest requests, for the staff list. A scoped caller reads its tenant's only.
SELECT * FROM quota_request
WHERE (NOT sqlc.arg('scoped')::boolean OR tenant_id = sqlc.arg('scope_tenant')::uuid)
  AND (sqlc.arg('state')::text = '' OR state = sqlc.arg('state')::text)
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
    )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: WorkspaceEffectiveLimit :one
-- The effective value of one plan gate for a workspace: its override when one is
-- set, else its plan's limit. unlimited is true when the value is NULL (no limit
-- or a gate the plan does not list). A workspace with no subscription row is on
-- the free plan.
SELECT (eff.v IS NULL)::boolean AS unlimited, COALESCE(eff.v, 0)::bigint AS value
FROM (
    SELECT (
        CASE
            WHEN COALESCE(ws.limit_overrides, '{}'::jsonb) ? sqlc.arg('gate')::text
                THEN ws.limit_overrides->>sqlc.arg('gate')::text
            ELSE p.limits->>sqlc.arg('gate')::text
        END
    )::bigint AS v
    FROM plan p
    LEFT JOIN workspace_subscription ws ON ws.workspace_id = sqlc.arg('workspace_id')::uuid
    WHERE p.key = COALESCE(ws.plan_key, 'free')
) AS eff;
