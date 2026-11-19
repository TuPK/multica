-- name: ListTenantWorkspaceIDs :many
-- Every workspace of the tenant the given workspace belongs to, itself included.
-- No rows when the workspace has no tenant link.
SELECT sib.workspace_id
FROM tenant_workspace own
JOIN tenant_workspace sib ON sib.tenant_id = own.tenant_id
WHERE own.workspace_id = $1
ORDER BY sib.workspace_id;

-- name: SearchTenants :many
-- The tenants a page of the console list shows, newest first. A scoped caller
-- (an admin) reads only its own tenant: scoped is true and scope_tenant is the
-- tenant, or null for none. The pattern matches the tenant's name, the name or
-- slug of one of its workspaces, or the email of one of their owners.
SELECT
    t.id,
    t.name,
    t.status,
    t.created_at,
    (SELECT count(*) FROM tenant_workspace tw WHERE tw.tenant_id = t.id)::bigint AS workspace_count,
    COALESCE(
        (SELECT max(e.created_at)
         FROM audit_event e
         JOIN tenant_workspace tw ON tw.workspace_id = e.workspace_id
         WHERE tw.tenant_id = t.id),
        t.created_at
    )::timestamptz AS last_activity_at
FROM tenant t
WHERE (NOT sqlc.arg('scoped')::boolean OR t.id = sqlc.narg('scope_tenant')::uuid)
  AND (
        sqlc.narg('pattern')::text IS NULL
        OR t.name ILIKE sqlc.narg('pattern')::text
        OR EXISTS (
            SELECT 1
            FROM tenant_workspace tw
            JOIN workspace w ON w.id = tw.workspace_id
            WHERE tw.tenant_id = t.id
              AND (
                    w.name ILIKE sqlc.narg('pattern')::text
                    OR w.slug ILIKE sqlc.narg('pattern')::text
                    OR EXISTS (
                        SELECT 1
                        FROM member o
                        JOIN "user" ou ON ou.id = o.user_id
                        WHERE o.workspace_id = w.id
                          AND o.role = 'owner'
                          AND ou.email ILIKE sqlc.narg('pattern')::text
                    )
              )
        )
    )
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (t.created_at, t.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
    )
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetTenant :one
SELECT t.id, t.name, t.status, t.created_by, t.created_at,
    (SELECT count(*) FROM tenant_workspace tw WHERE tw.tenant_id = t.id)::bigint AS workspace_count,
    COALESCE(
        (SELECT max(e.created_at)
         FROM audit_event e
         JOIN tenant_workspace tw ON tw.workspace_id = e.workspace_id
         WHERE tw.tenant_id = t.id),
        t.created_at
    )::timestamptz AS last_activity_at
FROM tenant t
WHERE t.id = sqlc.arg('id')
  AND (NOT sqlc.arg('scoped')::boolean OR t.id = sqlc.narg('scope_tenant')::uuid);

-- name: ListTenantWorkspaceSummaries :many
-- One row per workspace of the given tenants, with the plan it is on, its limit
-- overrides, the limits of its plan and its owner. A workspace with no
-- subscription row is on the free plan. The plan limits are null when the plan
-- row is missing.
SELECT
    tw.tenant_id,
    w.id AS workspace_id,
    w.name,
    w.slug,
    w.created_at,
    COALESCE(pw.status, 'active')::text AS status,
    COALESCE(s.plan_key, 'free')::text AS plan_key,
    COALESCE(s.limit_overrides, '{}'::jsonb) AS limit_overrides,
    p.limits AS plan_limits,
    COALESCE(p.sort, 0)::int AS plan_sort,
    (SELECT count(*) FROM member m WHERE m.workspace_id = w.id)::bigint AS member_count,
    ow.user_id AS owner_id,
    owner_user.email AS owner_email
FROM tenant_workspace tw
JOIN workspace w ON w.id = tw.workspace_id
LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
LEFT JOIN workspace_subscription s ON s.workspace_id = w.id
LEFT JOIN plan p ON p.key = COALESCE(s.plan_key, 'free')
LEFT JOIN LATERAL (
    SELECT m.user_id
    FROM member m
    WHERE m.workspace_id = w.id AND m.role = 'owner'
    ORDER BY m.created_at, m.id
    LIMIT 1
) ow ON true
LEFT JOIN "user" owner_user ON owner_user.id = ow.user_id
WHERE tw.tenant_id = ANY(sqlc.arg('tenant_ids')::uuid[])
ORDER BY tw.tenant_id, w.created_at, w.id;

-- name: WorkspaceInScope :one
-- Whether the workspace belongs to the scope's tenant.
SELECT EXISTS (
    SELECT 1 FROM tenant_workspace tw
    WHERE tw.workspace_id = sqlc.arg('workspace_id')
      AND tw.tenant_id = sqlc.narg('scope_tenant')::uuid
);

-- name: UserInScope :one
-- Whether the user is a member of a workspace of the scope's tenant.
SELECT EXISTS (
    SELECT 1
    FROM member m
    JOIN tenant_workspace tw ON tw.workspace_id = m.workspace_id
    WHERE m.user_id = sqlc.arg('user_id')
      AND tw.tenant_id = sqlc.narg('scope_tenant')::uuid
);

-- name: TenantExists :one
SELECT EXISTS (SELECT 1 FROM tenant WHERE id = $1);

-- name: CountWorkspacesWithoutTenant :one
SELECT count(*)::bigint
FROM workspace w
WHERE NOT EXISTS (SELECT 1 FROM tenant_workspace tw WHERE tw.workspace_id = w.id);

-- name: ListWorkspacesOfTenant :many
-- The workspace ids of one tenant, in a fixed order (spec R6 section 3b.4, rule 4).
SELECT workspace_id FROM tenant_workspace
WHERE tenant_id = $1
ORDER BY workspace_id;

-- name: TenantExistsForLimits :one
SELECT EXISTS (SELECT 1 FROM tenant WHERE id = $1);
