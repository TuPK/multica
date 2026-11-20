-- Legal holds (spec R6 section 3c.7). The platform writes legal_hold (migrations
-- 1038 and 1039); the deletion paths read it through the queries below.

-- name: ActiveLegalHoldForWorkspace :one
-- A live hold on the workspace, or on its tenant, which covers every workspace of
-- the tenant. Pass a NULL tenant_id for a workspace with no tenant.
SELECT id FROM legal_hold
WHERE released_at IS NULL
  AND (
        (scope_kind = 'workspace' AND scope_id = @workspace_id::uuid)
     OR (scope_kind = 'tenant' AND scope_id = sqlc.narg('tenant_id')::uuid)
  )
LIMIT 1;

-- name: ActiveLegalHoldForAccount :one
SELECT id FROM legal_hold
WHERE released_at IS NULL AND scope_kind = 'account' AND scope_id = $1
LIMIT 1;
