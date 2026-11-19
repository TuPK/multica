-- name: GetActiveStaffByUserID :one
SELECT * FROM platform_staff
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: GetUserIDByEmail :one
-- Multica's user table is the only table of Multica's that the platform reads
-- for sign-in. The caller passes the email lower-cased, as Multica stores it.
SELECT id FROM "user"
WHERE email = $1;

-- name: GetUserEmailByID :one
SELECT email FROM "user"
WHERE id = $1;

-- name: GetAccountState :one
SELECT status, sessions_valid_after FROM platform_account
WHERE user_id = $1;

-- name: InsertStaff :execrows
-- Does nothing when the user already has an active row.
INSERT INTO platform_staff (user_id, role, created_by, tenant_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) WHERE revoked_at IS NULL DO NOTHING;

-- name: ReactivateStaff :execrows
-- Re-activates the user's most recently created revoked row with the given
-- role and tenant (null unless the role is admin). It changes nothing when the user has no revoked row, and fails on the
-- unique index when the user already has an active row.
UPDATE platform_staff
SET revoked_at = NULL, role = $2, tenant_id = $3
WHERE ctid = (
    SELECT s.ctid FROM platform_staff s
    WHERE s.user_id = $1 AND s.revoked_at IS NOT NULL
    ORDER BY s.created_at DESC
    LIMIT 1
);

-- name: UpdateStaffRoleAndTenant :execrows
-- Changes the role and tenant of the user's active row.
UPDATE platform_staff
SET role = $2, tenant_id = $3
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeStaff :execrows
UPDATE platform_staff
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: ListStaff :many
-- Active rows only; revoked rows stay in the table as history.
SELECT * FROM platform_staff
WHERE revoked_at IS NULL
ORDER BY created_at, user_id;
