-- Staff add-member requests (spec R6 section 4): a staff member asks for a person
-- to be added to a workspace. The platform writes the request and cancels it;
-- Multica's member_provisioning job moves it on.

-- name: InsertMemberRequest :one
-- Inserts a pending request unless one is already live for the workspace, kind
-- and email. The conflict target is the partial unique index of migration 1024;
-- a lost race returns no row, and the caller reads the winner.
INSERT INTO platform_member_request (
    id, workspace_id, kind, target_user_id, previous_role, email, role, requested_by, requested_by_label
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (workspace_id, kind, email) WHERE state = 'pending' DO NOTHING
RETURNING *;

-- name: GetLiveMemberRequest :one
SELECT * FROM platform_member_request
WHERE workspace_id = $1 AND kind = $2 AND email = $3 AND state = 'pending';

-- name: GetMemberRequest :one
SELECT * FROM platform_member_request
WHERE id = $1;

-- name: LockMemberRequest :one
SELECT * FROM platform_member_request
WHERE id = $1
FOR UPDATE;

-- name: CancelMemberRequest :one
UPDATE platform_member_request
SET state = 'canceled', updated_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING *;

-- name: ListMemberRequestsByWorkspace :many
-- The newest requests of one workspace. The table is small and written by
-- people, so a page is the newest rows and needs no cursor.
SELECT * FROM platform_member_request
WHERE workspace_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: GetMemberRequestWorkspace :one
-- What the request path needs of a workspace: its name, platform status and
-- tenant. No row means the workspace does not exist.
SELECT w.id, w.name,
    COALESCE(pw.status, 'active')::text AS status,
    tw.tenant_id AS tenant_id
FROM workspace w
LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
LEFT JOIN tenant_workspace tw ON tw.workspace_id = w.id
WHERE w.id = $1;

-- name: IsActiveStaffEmail :one
-- Whether the email belongs to a staff member who is active now (spec R6 section
-- 4.2, rule 4). The email is lower-cased by the caller.
SELECT EXISTS (
    SELECT 1
    FROM platform_staff s
    JOIN "user" u ON u.id = s.user_id
    WHERE lower(u.email) = $1 AND s.revoked_at IS NULL
);

-- name: IsWorkspaceMemberEmail :one
SELECT EXISTS (
    SELECT 1
    FROM member m
    JOIN "user" u ON u.id = m.user_id
    WHERE m.workspace_id = $1 AND lower(u.email) = $2
);
