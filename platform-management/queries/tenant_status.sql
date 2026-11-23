-- Locking a whole tenant (spec R6 section 3c.9, rule 1). The platform writes the
-- tenant's status columns (migration 1016) and each workspace's platform status
-- through workspacestatus; Delegora reads both.

-- name: LockTenantForStatus :one
SELECT id, status FROM tenant
WHERE id = $1
FOR UPDATE;

-- name: TenantWorkspaceStatuses :many
-- Every workspace of the tenant with its platform status and the reason it has
-- (active and no reason when it has no platform row).
SELECT tw.workspace_id,
       COALESCE(pw.status, 'active')::text AS status,
       COALESCE(pw.reason, '')::text AS reason
FROM tenant_workspace tw
LEFT JOIN platform_workspace pw ON pw.workspace_id = tw.workspace_id
WHERE tw.tenant_id = $1
ORDER BY tw.workspace_id;

-- name: SetTenantStatus :exec
UPDATE tenant
SET status = $2,
    status_reason = $3,
    status_changed_at = now(),
    status_changed_by = $4,
    updated_at = now()
WHERE id = $1;
