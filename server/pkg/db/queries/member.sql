-- name: ListMembers :many
SELECT * FROM member
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: ListWorkspaceManagerUserIDs :many
SELECT user_id FROM member
WHERE workspace_id = $1 AND role IN ('owner', 'admin')
ORDER BY created_at ASC;

-- name: GetMember :one
SELECT * FROM member
WHERE id = $1;

-- name: GetMemberByUserAndWorkspace :one
SELECT * FROM member
WHERE user_id = $1 AND workspace_id = $2;

-- name: GetRealtimeMemberships :many
SELECT m.user_id, m.workspace_id
FROM member m
JOIN (
    SELECT unnest(sqlc.arg(user_ids)::uuid[]) AS user_id,
           unnest(sqlc.arg(workspace_ids)::uuid[]) AS workspace_id
) AS requested
    ON m.user_id = requested.user_id AND m.workspace_id = requested.workspace_id;

-- name: CreateMember :one
INSERT INTO member (workspace_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateGuestMember :one
INSERT INTO member (workspace_id, user_id, role, is_guest)
VALUES ($1, $2, $3, true)
RETURNING *;

-- name: UpdateMemberRole :one
UPDATE member SET role = $2
WHERE id = $1
RETURNING *;

-- name: UpdateMemberGuest :one
UPDATE member SET is_guest = $2 WHERE id = $1 RETURNING *;

-- name: DeleteMember :exec
DELETE FROM member WHERE id = $1;

-- name: ListMembersWithUser :many
SELECT m.id, m.workspace_id, m.user_id, m.role, m.created_at,
       u.name as user_name, u.email as user_email, u.avatar_url as user_avatar_url
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = $1
ORDER BY m.created_at ASC;

-- name: RecordMemberRemoval :exec
INSERT INTO member_removal (workspace_id, user_id) VALUES ($1, $2);

-- member_removal has no foreign key, so workspace deletion removes its rows here.
-- name: DeleteMemberRemovalsByWorkspace :exec
DELETE FROM member_removal WHERE workspace_id = $1;
