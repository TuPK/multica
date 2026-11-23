-- The staff console's reads and writes. They touch only the tables below:
-- the Delegora account and workspace tables, the audit log, and the platform's
-- own tables. A scan test (internal/console) fails when this file names any
-- other table.
--
-- Searches take a pattern the caller has already escaped for LIKE and wrapped
-- in percent signs. Lists are newest first, keyset-paged on (created_at, id);
-- callers ask for one row more than they return to learn whether another page
-- exists.

-- name: GetWorkspace :one
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
        NOT sqlc.arg('scoped')::boolean
        OR EXISTS (
            SELECT 1
            FROM member sm
            JOIN tenant_workspace stw ON stw.workspace_id = sm.workspace_id
            WHERE sm.user_id = u.id AND stw.tenant_id = sqlc.narg('scope_tenant')::uuid
        )
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

-- name: LockActiveSystemAdmins :many
-- Locks every active system administrator row, so two removals or demotions
-- cannot each see the other administrator still in place.
SELECT user_id FROM platform_staff
WHERE role = 'system_admin' AND revoked_at IS NULL
ORDER BY user_id
FOR UPDATE;

-- name: ListActiveStaffWithEmail :many
SELECT s.user_id, u.email, s.role, s.tenant_id, s.created_by, s.created_at
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
-- Every workspace's rows and the account-level rows. A scoped caller (an admin)
-- reads the rows of its tenant: those that carry its tenant_id, and those of its
-- workspaces (spec R6 section 3.5, D84).
SELECT * FROM audit_event
WHERE (
        NOT sqlc.arg('scoped')::boolean
        OR tenant_id = sqlc.narg('scope_tenant')::uuid
        OR workspace_id IN (SELECT tw.workspace_id FROM tenant_workspace tw WHERE tw.tenant_id = sqlc.narg('scope_tenant')::uuid)
    )
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

-- name: CountWorkspacesCreatedByDay :many
-- New workspaces per UTC day since the instant given. A day with none is absent;
-- the reader fills it with zero.
SELECT (w.created_at AT TIME ZONE 'UTC')::date AS day, count(*)::bigint AS total
FROM workspace w
WHERE w.created_at >= sqlc.arg('since')::timestamptz
GROUP BY 1;

-- name: CountBillingOrdersCreatedByDay :many
-- Billing orders created per UTC day since the instant given, whatever their status.
SELECT (o.created_at AT TIME ZONE 'UTC')::date AS day, count(*)::bigint AS total
FROM billing_order o
WHERE o.created_at >= sqlc.arg('since')::timestamptz
GROUP BY 1;

-- name: CountBillingOrdersPaidByDay :many
-- Billing orders paid per UTC day of their payment since the instant given. The
-- order states have no "confirmed" value: a confirmed payment is the paid state,
-- with paid_at set.
SELECT (o.paid_at AT TIME ZONE 'UTC')::date AS day, count(*)::bigint AS total
FROM billing_order o
WHERE o.status = 'paid' AND o.paid_at >= sqlc.arg('since')::timestamptz
GROUP BY 1;

-- name: CountOperationsQueue :one
-- The items waiting in the operations queue, counted as the list screens filter
-- them: approvals are the deletion requests awaiting approval, deletions the
-- pending ones; failed e-invoices are the records in the failed state; active
-- legal holds are the holds not yet released.
SELECT
    (SELECT count(*) FROM platform_deletion_request d WHERE d.state = 'awaiting_approval')::bigint AS pending_approvals,
    (SELECT count(*) FROM platform_deletion_request d WHERE d.state = 'pending')::bigint AS pending_deletions,
    (SELECT count(*) FROM quota_request q WHERE q.state = 'pending')::bigint AS pending_quota_requests,
    (SELECT count(*) FROM einvoice_issuance e WHERE e.state = 'failed')::bigint AS failed_einvoices,
    (SELECT count(*) FROM legal_hold h WHERE h.released_at IS NULL)::bigint AS active_legal_holds;

-- name: ListUserIDsWithLogin :many
-- The users, among the ids given, with an auth.login audit row. The (actor_id,
-- created_at) index serves the actor_id filter.
SELECT DISTINCT actor_id::text AS user_id
FROM audit_event
WHERE action = 'auth.login' AND actor_id = ANY(sqlc.arg('user_ids')::text[]);

-- name: SearchWorkspaces :many
-- The workspaces of the staff list, newest first (spec R6 section 3b.2). The
-- pattern matches name, slug, owner email and tenant name. A workspace without
-- a subscription is on the free plan, as the billing path reads it.
SELECT
    w.id,
    w.name,
    w.slug,
    w.created_at,
    COALESCE(pw.status, 'active')::text AS status,
    COALESCE(ws.plan_key, 'free')::text AS plan_key,
    tw.tenant_id,
    COALESCE(tn.name, '')::text AS tenant_name,
    (SELECT count(*) FROM member m WHERE m.workspace_id = w.id)::bigint AS member_count,
    COALESCE(owner_user.email, '')::text AS owner_email
FROM workspace w
LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
LEFT JOIN workspace_subscription ws ON ws.workspace_id = w.id
LEFT JOIN tenant_workspace tw ON tw.workspace_id = w.id
LEFT JOIN tenant tn ON tn.id = tw.tenant_id
LEFT JOIN LATERAL (
    SELECT m.user_id
    FROM member m
    WHERE m.workspace_id = w.id AND m.role = 'owner'
    ORDER BY m.created_at, m.id
    LIMIT 1
) ow ON true
LEFT JOIN "user" owner_user ON owner_user.id = ow.user_id
WHERE (
        sqlc.narg('pattern')::text IS NULL
        OR w.name ILIKE sqlc.narg('pattern')::text
        OR w.slug ILIKE sqlc.narg('pattern')::text
        OR owner_user.email ILIKE sqlc.narg('pattern')::text
        OR tn.name ILIKE sqlc.narg('pattern')::text
    )
  AND (sqlc.arg('status')::text = '' OR COALESCE(pw.status, 'active') = sqlc.arg('status')::text)
  AND (sqlc.arg('plan_key')::text = '' OR COALESCE(ws.plan_key, 'free') = sqlc.arg('plan_key')::text)
  AND (NOT sqlc.arg('scoped')::boolean OR tw.tenant_id = sqlc.arg('scope_tenant')::uuid)
  AND (sqlc.narg('tenant_filter')::uuid IS NULL OR tw.tenant_id = sqlc.narg('tenant_filter')::uuid)
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (w.created_at, w.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
    )
ORDER BY w.created_at DESC, w.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetWorkspaceTenantID :one
-- The tenant a workspace belongs to. No row when it has none.
SELECT tenant_id FROM tenant_workspace
WHERE workspace_id = $1;

