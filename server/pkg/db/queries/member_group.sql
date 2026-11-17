-- Account groups (platform management spec 7b.2). Every query filters by
-- workspace_id, so the id of another workspace's group finds nothing.

-- name: CreateMemberGroup :one
INSERT INTO member_group (workspace_id, name, description, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMemberGroup :one
SELECT * FROM member_group WHERE workspace_id = $1 AND id = $2;

-- name: GetMemberGroupForUpdate :one
-- Serialises the changes of one group, so a member count read under it stays
-- exact until the transaction ends.
SELECT * FROM member_group WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: ListMemberGroups :many
SELECT g.*,
       (SELECT count(*) FROM member_group_member m
         WHERE m.workspace_id = g.workspace_id AND m.group_id = g.id)::bigint AS member_count
FROM member_group g
WHERE g.workspace_id = $1
ORDER BY lower(g.name), g.id;

-- name: CountMemberGroups :one
SELECT count(*)::bigint FROM member_group WHERE workspace_id = $1;

-- name: UpdateMemberGroup :one
UPDATE member_group
SET name = $3, description = $4, updated_at = now()
WHERE workspace_id = $1 AND id = $2
RETURNING *;

-- name: UpdateMemberGroupPermissions :one
UPDATE member_group
SET permissions = $3, updated_at = now()
WHERE workspace_id = $1 AND id = $2
RETURNING *;

-- name: DeleteMemberGroup :execrows
DELETE FROM member_group WHERE workspace_id = $1 AND id = $2;

-- name: DeleteMemberGroupMembers :execrows
DELETE FROM member_group_member WHERE workspace_id = $1 AND group_id = $2;

-- name: ListMemberGroupMembers :many
SELECT * FROM member_group_member
WHERE workspace_id = $1 AND group_id = $2
ORDER BY added_at, user_id;

-- name: CountMemberGroupMembers :one
SELECT count(*)::bigint FROM member_group_member WHERE workspace_id = $1 AND group_id = $2;

-- name: ListWorkspaceMemberUserIDs :many
-- The ones of user_ids that are members of the workspace.
SELECT user_id FROM member WHERE workspace_id = $1 AND user_id = ANY(@user_ids::uuid[]);

-- name: ListMemberGroupUserIDs :many
-- The ones of user_ids that are already in the group.
SELECT user_id FROM member_group_member
WHERE workspace_id = $1 AND group_id = $2 AND user_id = ANY(@user_ids::uuid[]);

-- name: AddMemberGroupMembers :many
-- Returns the people actually added; one already in the group is skipped.
INSERT INTO member_group_member (workspace_id, group_id, user_id, added_by)
SELECT @workspace_id, @group_id, u, @added_by FROM unnest(@user_ids::uuid[]) AS u
ON CONFLICT DO NOTHING
RETURNING user_id;

-- name: DeleteMemberGroupMember :execrows
DELETE FROM member_group_member WHERE workspace_id = $1 AND group_id = $2 AND user_id = $3;

-- name: DeleteMemberGroupMembersByUser :exec
-- Drops one person's group rows in a workspace, in the transaction that removes
-- the member.
DELETE FROM member_group_member WHERE workspace_id = $1 AND user_id = $2;

-- name: GetMemberPermissionInputs :one
-- The role of a member and the permission arrays of every group they are in,
-- in one statement. No row means the user is not a member of the workspace.
SELECT m.role,
       COALESCE((
           SELECT array_agg(g.permissions)
           FROM member_group_member gm
           JOIN member_group g ON g.workspace_id = gm.workspace_id AND g.id = gm.group_id
           WHERE gm.workspace_id = m.workspace_id AND gm.user_id = m.user_id
       ), '{}')::jsonb[] AS group_permissions
FROM member m
WHERE m.workspace_id = $1 AND m.user_id = $2;
