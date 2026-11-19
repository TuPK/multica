-- The tenant backfill's batches (spec R6 section 5.4, step 5). Each statement is
-- safe to run twice and at the same time as another run: the unique indexes on
-- tenant (id) and tenant_workspace (workspace_id) absorb the conflicts.

-- name: ListWorkspacesWithoutTenant :many
-- The next batch of workspaces with no tenant link, in id order.
SELECT w.id, w.name
FROM workspace w
WHERE NOT EXISTS (
    SELECT 1 FROM tenant_workspace tw WHERE tw.workspace_id = w.id
)
ORDER BY w.id
LIMIT $1;

-- name: InsertTenantOfWorkspace :execrows
-- The tenant keyed by the workspace id, named and dated after it.
INSERT INTO tenant (id, name, created_at, updated_at)
SELECT w.id, w.name, w.created_at, w.created_at
FROM workspace w
WHERE w.id = $1
ON CONFLICT (id) DO NOTHING;

-- name: LinkWorkspaceToOwnTenant :execrows
INSERT INTO tenant_workspace (workspace_id, tenant_id, created_at)
SELECT w.id, w.id, w.created_at
FROM workspace w
WHERE w.id = $1
ON CONFLICT (workspace_id) DO NOTHING;
