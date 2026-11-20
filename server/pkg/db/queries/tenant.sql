-- name: GetTenantIDByWorkspace :one
-- The tenant a workspace belongs to. No row means the workspace has no link yet.
SELECT tenant_id FROM tenant_workspace WHERE workspace_id = $1;

-- name: DeleteTenantWorkspaceLink :exec
DELETE FROM tenant_workspace WHERE workspace_id = $1;

-- name: CountTenantWorkspaceLinks :one
SELECT count(*)::bigint FROM tenant_workspace WHERE tenant_id = $1;

-- name: DeleteTenant :exec
DELETE FROM tenant WHERE id = $1;

-- name: CreateTenant :exec
-- A tenant keyed by the id of the workspace that makes it, named after it.
INSERT INTO tenant (id, name, created_by) VALUES ($1, $2, $3);

-- name: LinkWorkspaceToTenant :exec
INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2);

-- name: ListOwnedTenantWorkspaceIDs :many
-- The workspaces of the tenant that the user owns, oldest first. The first one
-- is the workspace whose entitlement policy CreateWorkspace reads.
SELECT w.id
FROM tenant_workspace tw
JOIN workspace w ON w.id = tw.workspace_id
JOIN member m ON m.workspace_id = w.id AND m.user_id = $2 AND m.role = 'owner'
WHERE tw.tenant_id = $1
ORDER BY w.created_at, w.id;

-- name: ListTenantWorkspaceIDs :many
-- The workspaces linked to a tenant. A staff tenant deletion walks them one by one.
SELECT workspace_id FROM tenant_workspace WHERE tenant_id = $1 ORDER BY workspace_id;

-- name: GetTenantStatus :one
-- The status the platform set on the tenant: active, read_only or suspended (spec R6 3c.9).
SELECT status FROM tenant WHERE id = $1;
