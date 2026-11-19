-- Legal holds (spec R6 section 3c.7, QT11). The platform writes legal_hold
-- (migrations 1038 and 1039); deletion paths read it through the checks below.

-- name: InsertLegalHold :one
-- Places a hold. A live hold on the same scope makes the insert return no row.
INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref, placed_by, placed_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now())
ON CONFLICT (scope_kind, scope_id) WHERE released_at IS NULL DO NOTHING
RETURNING *;

-- name: ReleaseLegalHold :one
-- Releases a live hold. A released or unknown hold matches nothing.
UPDATE legal_hold
SET released_by = $2, released_at = now()
WHERE id = $1 AND released_at IS NULL
RETURNING *;

-- name: GetLegalHold :one
SELECT * FROM legal_hold WHERE id = $1;

-- name: ListLegalHolds :many
-- Live holds first, then released ones, newest first.
SELECT * FROM legal_hold
ORDER BY (released_at IS NULL) DESC, placed_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: ActiveLegalHoldForWorkspace :one
-- A live hold on the workspace, or on its tenant (the hold covers every workspace
-- of the tenant). No row when there is none.
SELECT id FROM legal_hold
WHERE released_at IS NULL
  AND (
        (scope_kind = 'workspace' AND scope_id = sqlc.arg('workspace_id')::uuid)
     OR (scope_kind = 'tenant' AND scope_id = sqlc.narg('tenant_id')::uuid)
  )
LIMIT 1;

-- name: ActiveLegalHoldForTenant :one
SELECT id FROM legal_hold
WHERE released_at IS NULL AND scope_kind = 'tenant' AND scope_id = $1
LIMIT 1;

-- name: ActiveLegalHoldForAccount :one
SELECT id FROM legal_hold
WHERE released_at IS NULL AND scope_kind = 'account' AND scope_id = $1
LIMIT 1;
