-- Users of a tenant (spec R6 section 3b.3, QT3). A user belongs to a tenant
-- when it is a member of at least one of the tenant's workspaces (D92). The
-- platform reads the Multica member and user tables and writes only its own
-- request table (through InsertMemberRequest).

-- name: ListTenantUserRows :many
-- The memberships of the tenant's users, one row per (user, workspace), for
-- the caller to group. Users are keyset-paged by id: the page is the first
-- sqlc.arg(row_limit) distinct users after cursor_user_id, and every membership
-- of those users comes back. Staff hide from admin through the scoped flag.
WITH page AS (
    SELECT DISTINCT m.user_id
    FROM member m
    JOIN tenant_workspace tw ON tw.workspace_id = m.workspace_id
    JOIN "user" u ON u.id = m.user_id
    WHERE tw.tenant_id = sqlc.arg('tenant_id')::uuid
      AND (sqlc.arg('pattern')::text = '' OR u.email ILIKE sqlc.arg('pattern')::text OR u.name ILIKE sqlc.arg('pattern')::text)
      AND (sqlc.arg('cursor_user_id')::text = '' OR m.user_id > sqlc.arg('cursor_user_id')::text::uuid)
    ORDER BY m.user_id
    LIMIT sqlc.arg('row_limit')
)
SELECT
    u.id AS user_id,
    u.email,
    u.name,
    COALESCE(pa.status, 'active')::text AS account_status,
    EXISTS (SELECT 1 FROM platform_staff s WHERE s.user_id = u.id AND s.revoked_at IS NULL) AS is_staff,
    w.id AS workspace_id,
    w.name AS workspace_name,
    m.role AS member_role,
    COALESCE((SELECT max(e.created_at) FROM audit_event e WHERE e.action = 'auth.login' AND e.actor_id = u.id::text), NULL)::timestamptz AS last_login_at
FROM page
JOIN "user" u ON u.id = page.user_id
JOIN member m ON m.user_id = u.id
JOIN workspace w ON w.id = m.workspace_id
JOIN tenant_workspace tw ON tw.workspace_id = w.id AND tw.tenant_id = sqlc.arg('tenant_id')::uuid
LEFT JOIN platform_account pa ON pa.user_id = u.id
ORDER BY u.id, w.name, w.id;

-- name: GetTenantUserMemberships :many
-- One user's memberships in the tenant's workspaces.
SELECT
    u.id AS user_id,
    u.email,
    u.name,
    COALESCE(pa.status, 'active')::text AS account_status,
    EXISTS (SELECT 1 FROM platform_staff s WHERE s.user_id = u.id AND s.revoked_at IS NULL) AS is_staff,
    w.id AS workspace_id,
    w.name AS workspace_name,
    m.role AS member_role
FROM "user" u
JOIN member m ON m.user_id = u.id
JOIN workspace w ON w.id = m.workspace_id
JOIN tenant_workspace tw ON tw.workspace_id = w.id AND tw.tenant_id = $1
LEFT JOIN platform_account pa ON pa.user_id = u.id
WHERE u.id = $2
ORDER BY w.name, w.id;

-- name: GetWorkspaceMemberRole :one
-- A user's role in one workspace, with the user's email. No row when not a member.
SELECT m.role, u.email
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = $1 AND m.user_id = $2;
