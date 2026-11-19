-- The tenant policy floor (spec R6 section 3c.4, D101). The platform writes
-- tenant_policy (migration 1033, unique index 1034); Multica reads it. The
-- feature check reads the workspaces' subscriptions and the plan catalogue.

-- name: GetTenantPolicy :one
SELECT * FROM tenant_policy
WHERE tenant_id = $1;

-- name: GetTenantPolicyForUpdate :one
-- Locks the tenant's policy row, so an expected version is checked against the write.
SELECT * FROM tenant_policy
WHERE tenant_id = $1
FOR UPDATE;

-- name: UpsertTenantPolicy :exec
INSERT INTO tenant_policy (
    tenant_id, allowed_invite_domains, share_links_enabled, require_sso, require_mfa,
    session_max_age_hours, version, updated_by, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, now()
)
ON CONFLICT (tenant_id) DO UPDATE
SET allowed_invite_domains = EXCLUDED.allowed_invite_domains,
    share_links_enabled = EXCLUDED.share_links_enabled,
    require_sso = EXCLUDED.require_sso,
    require_mfa = EXCLUDED.require_mfa,
    session_max_age_hours = EXCLUDED.session_max_age_hours,
    version = EXCLUDED.version,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();

-- name: TenantHasPolicyFeature :one
-- True when a workspace of the tenant is on a plan with the workspace_policies
-- feature. A workspace with no subscription row is on the free plan.
SELECT EXISTS (
    SELECT 1
    FROM tenant_workspace tw
    LEFT JOIN workspace_subscription ws ON ws.workspace_id = tw.workspace_id
    JOIN plan p ON p.key = COALESCE(ws.plan_key, 'free')
    WHERE tw.tenant_id = $1
      AND COALESCE((p.features->>'workspace_policies')::boolean, false)
);
