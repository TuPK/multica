-- name: GetViewerMember :one
SELECT role, is_guest FROM member
WHERE workspace_id = $1 AND user_id = $2;

-- name: ListViewerProjectAccess :many
-- The projects whose visibility differs from "everyone in the workspace sees
-- it": every project that is not exactly 'workspace' (an unknown value is
-- restricted), plus every project the viewer is granted.
SELECT p.id,
       (p.access <> 'workspace')::boolean AS restricted,
       EXISTS (
           SELECT 1 FROM project_member pm
           WHERE pm.project_id = p.id AND pm.user_id = @user_id
       )::boolean AS granted
FROM project p
WHERE p.workspace_id = @workspace_id
  AND (
      p.access <> 'workspace'
      OR EXISTS (
          SELECT 1 FROM project_member pm
          WHERE pm.project_id = p.id AND pm.user_id = @user_id
      )
  );

-- name: GetProjectAccess :one
SELECT id, workspace_id, title, access, creator_type, creator_id, lead_type, lead_id
FROM project
WHERE id = $1 AND workspace_id = $2;

-- name: LockProjectAccess :one
-- Serializes concurrent access switches on one project.
SELECT id, workspace_id, title, access, creator_type, creator_id, lead_type, lead_id
FROM project
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

-- name: SetProjectAccess :exec
UPDATE project SET access = $3
WHERE id = $1 AND workspace_id = $2;

-- name: CountProjectIssues :one
SELECT count(*) FROM issue
WHERE project_id = $1 AND workspace_id = $2;

-- name: FilterWorkspaceMemberUserIDs :many
SELECT user_id FROM member
WHERE workspace_id = $1 AND user_id = ANY(@user_ids::uuid[]);

-- name: GrantProjectMembers :many
INSERT INTO project_member (project_id, workspace_id, user_id, added_by)
SELECT @project_id, @workspace_id, u, @added_by
FROM unnest(@user_ids::uuid[]) AS u
ON CONFLICT (project_id, user_id) DO NOTHING
RETURNING user_id;

-- name: GrantProjectMembersFromProject :execrows
-- Grants the people who would otherwise lose their working place when a
-- project becomes restricted: its creator, its lead and the assignees of its
-- issues. Only current workspace members are granted.
INSERT INTO project_member (project_id, workspace_id, user_id, added_by)
SELECT @project_id, @workspace_id, m.user_id, @added_by
FROM member m
WHERE m.workspace_id = @workspace_id
  AND m.user_id IN (
      SELECT p.creator_id FROM project p
      WHERE p.id = @project_id AND p.creator_type = 'member'
      UNION
      SELECT p.lead_id FROM project p
      WHERE p.id = @project_id AND p.lead_type = 'member'
      UNION
      SELECT i.assignee_id FROM issue i
      WHERE i.project_id = @project_id AND i.assignee_type = 'member'
  )
ON CONFLICT (project_id, user_id) DO NOTHING;

-- name: RevokeProjectMember :execrows
DELETE FROM project_member
WHERE project_id = $1 AND user_id = $2;

-- name: ListProjectMembers :many
SELECT pm.user_id, pm.added_by, pm.created_at, u.name
FROM project_member pm
JOIN "user" u ON u.id = pm.user_id
WHERE pm.project_id = $1
ORDER BY pm.created_at ASC, pm.user_id ASC;

-- name: SetMemberGuest :exec
UPDATE member SET is_guest = $3
WHERE workspace_id = $1 AND user_id = $2;
