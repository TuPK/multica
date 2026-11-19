-- name: GetTenantPolicy :one
-- The policy floor of a tenant. No row means the tenant sets no floor.
SELECT * FROM tenant_policy WHERE tenant_id = $1;

-- name: DeleteTenantPolicy :exec
-- The floor goes with the tenant when its last workspace is removed.
DELETE FROM tenant_policy WHERE tenant_id = $1;
