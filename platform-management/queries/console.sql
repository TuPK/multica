-- The staff console's reads and writes. They touch only the tables below:
-- the Multica account and workspace tables, the audit log, and the platform's
-- own tables. A scan test (internal/console) fails when this file names any
-- other table.
--
-- Searches take a pattern the caller has already escaped for LIKE and wrapped
-- in percent signs. Lists are newest first, keyset-paged on (created_at, id);
-- callers ask for one row more than they return to learn whether another page
-- exists.

-- name: SearchTenants :many
SELECT
    w.id,
    w.name,
    w.slug,
    COALESCE(pw.status, 'active')::text AS status,
    (SELECT count(*) FROM member m WHERE m.workspace_id = w.id)::bigint AS member_count,
    w.created_at,
    COALESCE(
        (SELECT max(e.created_at) FROM audit_event e WHERE e.workspace_id = w.id),
        w.created_at
    )::timestamptz AS last_activity_at
FROM workspace w
LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
WHERE (
        sqlc.narg('pattern')::text IS NULL
        OR w.name ILIKE sqlc.narg('pattern')::text
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
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (w.created_at, w.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
    )
ORDER BY w.created_at DESC, w.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetTenant :one
SELECT
    w.id,
    w.name,
    w.slug,
    COALESCE(pw.status, 'active')::text AS status,
    pw.reason AS state_reason,
    pw.changed_by AS state_changed_by,
    pw.changed_at AS state_changed_at,
    pw.deletion_scheduled_at,
    (SELECT count(*) FROM member m WHERE m.workspace_id = w.id)::bigint AS member_count,
    w.created_at,
    COALESCE(
        (SELECT max(e.created_at) FROM audit_event e WHERE e.workspace_id = w.id),
        w.created_at
    )::timestamptz AS last_activity_at,
    (SELECT count(*) FROM platform_note n WHERE n.workspace_id = w.id)::bigint AS notes_count,
    ow.user_id AS owner_id,
    owner_user.email AS owner_email
FROM workspace w
LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
LEFT JOIN LATERAL (
    SELECT m.user_id
    FROM member m
    WHERE m.workspace_id = w.id AND m.role = 'owner'
    ORDER BY m.created_at, m.id
    LIMIT 1
) ow ON true
LEFT JOIN "user" owner_user ON owner_user.id = ow.user_id
WHERE w.id = $1;

-- name: GetWorkspaceName :one
SELECT name FROM workspace
WHERE id = $1;

-- name: SearchUsers :many
SELECT
    u.id,
    u.email,
    u.name,
    COALESCE(pa.status, 'active')::text AS status,
    (SELECT count(*) FROM member m WHERE m.user_id = u.id)::bigint AS memberships_count,
    u.created_at
FROM "user" u
LEFT JOIN platform_account pa ON pa.user_id = u.id
WHERE (
        sqlc.narg('pattern')::text IS NULL
        OR u.email ILIKE sqlc.narg('pattern')::text
        OR u.name ILIKE sqlc.narg('pattern')::text
    )
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (u.created_at, u.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
    )
ORDER BY u.created_at DESC, u.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetUser :one
SELECT
    u.id,
    u.email,
    u.name,
    COALESCE(pa.status, 'active')::text AS status,
    (SELECT count(*) FROM member m WHERE m.user_id = u.id)::bigint AS memberships_count,
    u.created_at,
    pa.reason AS account_reason,
    pa.changed_at AS account_changed_at,
    pa.sessions_valid_after
FROM "user" u
LEFT JOIN platform_account pa ON pa.user_id = u.id
WHERE u.id = $1;

-- name: ListUserMemberships :many
SELECT w.id AS workspace_id, w.name AS workspace_name, m.role
FROM member m
JOIN workspace w ON w.id = m.workspace_id
WHERE m.user_id = $1
ORDER BY m.created_at, m.id;

-- name: GetPlatformAccountForUpdate :one
-- Locks the account row for the rest of the transaction. A missing row locks
-- nothing; the unique index on user_id settles two concurrent first writes.
SELECT * FROM platform_account
WHERE user_id = $1
FOR UPDATE;

-- name: UpsertAccountStatus :exec
-- Writes the status, reason and actor. sessions_valid_after and
-- deletion_scheduled_at of an existing row stay as they are.
INSERT INTO platform_account (user_id, status, reason, changed_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE
SET status = EXCLUDED.status,
    reason = EXCLUDED.reason,
    changed_by = EXCLUDED.changed_by,
    changed_at = now();

-- name: UpsertAccountSessionsValidAfter :exec
-- Sets only the revocation mark. An account with no row gets an active one.
INSERT INTO platform_account (user_id, status, reason, changed_by, sessions_valid_after)
VALUES ($1, 'active', '', $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET sessions_valid_after = EXCLUDED.sessions_valid_after;

-- name: LockActiveAdmins :many
-- Locks every active administrator row, so two removals cannot each see the
-- other administrator still in place.
SELECT user_id FROM platform_staff
WHERE role = 'admin' AND revoked_at IS NULL
ORDER BY user_id
FOR UPDATE;

-- name: ListActiveStaffWithEmail :many
SELECT s.user_id, u.email, s.role, s.created_by, s.created_at
FROM platform_staff s
JOIN "user" u ON u.id = s.user_id
WHERE s.revoked_at IS NULL
ORDER BY s.created_at, s.user_id;

-- name: ListWorkspaceAuditEvents :many
-- from is inclusive, to is exclusive.
SELECT * FROM audit_event
WHERE workspace_id = $1
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('actor_id')::text IS NULL OR actor_id = sqlc.narg('actor_id')::text)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR created_at < sqlc.narg('created_to')::timestamptz)
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: ListAllAuditEvents :many
-- Every workspace's rows and the account-level rows.
SELECT * FROM audit_event
WHERE (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('actor_id')::text IS NULL OR actor_id = sqlc.narg('actor_id')::text)
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR created_at >= sqlc.narg('created_from')::timestamptz)
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR created_at < sqlc.narg('created_to')::timestamptz)
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: CountWorkspaces :one
SELECT count(*)::bigint FROM workspace;

-- name: CountNonActiveWorkspacesByStatus :many
-- Only workspaces that still exist; the platform row has no foreign key.
SELECT pw.status, count(*)::bigint AS workspaces
FROM platform_workspace pw
JOIN workspace w ON w.id = pw.workspace_id
WHERE pw.status <> 'active'
GROUP BY pw.status;

-- name: CountSignupsSince :one
SELECT count(*)::bigint FROM "user"
WHERE created_at >= $1;

-- name: ListUserIDsWithLogin :many
-- The users, among the ids given, with an auth.login audit row. The (actor_id,
-- created_at) index serves the actor_id filter.
SELECT DISTINCT actor_id::text AS user_id
FROM audit_event
WHERE action = 'auth.login' AND actor_id = ANY(sqlc.arg('user_ids')::text[]);
