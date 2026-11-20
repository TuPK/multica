-- name: ListProjects :many
SELECT * FROM project
WHERE workspace_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('priority')::text IS NULL OR priority = sqlc.narg('priority'))
ORDER BY created_at DESC;

-- name: GetProjectInWorkspace :one
SELECT * FROM project
WHERE id = $1 AND workspace_id = $2;

-- name: LockProjectForChatSessionCreate :one
-- Conflicts with project deletion so a chat session cannot commit a soft
-- project reference after the delete transaction has swept existing sessions.
SELECT id FROM project
WHERE id = $1 AND workspace_id = $2
FOR KEY SHARE;

-- name: LockProjectForDelete :one
-- Serializes project deletion with chat-session creation. The handler locks,
-- clears every soft chat reference, and deletes the project in one transaction.
SELECT id FROM project
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

-- name: CreateProject :one
INSERT INTO project (
    workspace_id, title, description, icon, status,
    lead_type, lead_id, priority, start_date, due_date, workflow_id,
    creator_type, creator_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
) RETURNING *;

-- name: UpdateProject :one
UPDATE project SET
    title = COALESCE(sqlc.narg('title'), title),
    description = CASE WHEN sqlc.arg('description_set')::boolean THEN sqlc.narg('description') ELSE description END,
    icon = CASE WHEN sqlc.arg('icon_set')::boolean THEN sqlc.narg('icon') ELSE icon END,
    status = COALESCE(sqlc.narg('status'), status),
    priority = COALESCE(sqlc.narg('priority'), priority),
    lead_type = CASE WHEN sqlc.arg('lead_type_set')::boolean THEN sqlc.narg('lead_type') ELSE lead_type END,
    lead_id = CASE WHEN sqlc.arg('lead_id_set')::boolean THEN sqlc.narg('lead_id') ELSE lead_id END,
    start_date = CASE WHEN sqlc.arg('start_date_set')::boolean THEN sqlc.narg('start_date') ELSE start_date END,
    due_date = CASE WHEN sqlc.arg('due_date_set')::boolean THEN sqlc.narg('due_date') ELSE due_date END,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProject :exec
-- Defense-in-depth: workspace_id is a SQL-layer tenant guard. See DeleteIssue.
DELETE FROM project WHERE id = $1 AND workspace_id = $2;

-- name: CountIssuesByProject :one
SELECT count(*) FROM issue
WHERE issue.project_id = sqlc.arg('project_id')
-- projectaccess:scope
AND (sqlc.arg(scope_mode)::text = 'all'
    OR (issue.project_id IS NULL AND sqlc.arg(scope_mode)::text <> 'guest')
    OR EXISTS (SELECT 1 FROM project_member scope_pm
               WHERE scope_pm.project_id = issue.project_id AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
    OR (sqlc.arg(scope_mode)::text = 'member'
        AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = issue.project_id AND scope_p.access = 'workspace')));

-- name: GetProjectIssueStats :many
SELECT project_id,
       count(*)::bigint AS total_count,
       count(*) FILTER (WHERE status = ANY(sqlc.arg('terminal_status_keys')::text[]))::bigint AS done_count
FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND project_id = ANY(sqlc.arg('project_ids')::uuid[])
GROUP BY project_id;

-- name: SetProjectSquadIDs :one
-- Replaces the project's squad list. The handler computes it under the project's row lock.
UPDATE project SET squad_ids = sqlc.arg('squad_ids')::uuid[], updated_at = now()
WHERE id = sqlc.arg('id') AND workspace_id = sqlc.arg('workspace_id')
RETURNING *;

-- name: GetOpenSquadProject :one
-- An open project the squad is assigned to. A squad works one open project at a time.
SELECT * FROM project
WHERE workspace_id = sqlc.arg('workspace_id')
  AND sqlc.arg('squad_id')::uuid = ANY(squad_ids) AND id <> sqlc.arg('id')
  AND status NOT IN ('completed', 'cancelled')
LIMIT 1;

-- name: GetSquadWorkProjectID :one
-- The open project a squad holds, which is where its work and its repository are.
-- A squad works one open project at a time; the oldest wins if that ever breaks.
SELECT id FROM project
WHERE workspace_id = sqlc.arg('workspace_id')
  AND sqlc.arg('squad_id')::uuid = ANY(squad_ids)
  AND status NOT IN ('completed', 'cancelled')
ORDER BY created_at ASC, id ASC
LIMIT 1;

-- name: ClearSquadFromOtherProjects :many
-- Removes the squad from its other projects that the viewer sees. Only finished ones can remain once GetOpenSquadProject is clear.
UPDATE project SET squad_ids = array_remove(squad_ids, sqlc.arg('squad_id')::uuid), updated_at = now()
WHERE project.workspace_id = sqlc.arg('workspace_id')
  AND sqlc.arg('squad_id')::uuid = ANY(project.squad_ids) AND project.id <> sqlc.arg('id')
-- projectaccess:scope
  AND (sqlc.arg(scope_mode)::text = 'all'
      OR EXISTS (SELECT 1 FROM project_member scope_pm
                 WHERE scope_pm.project_id = project.id AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
      OR (sqlc.arg(scope_mode)::text = 'member'
          AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = project.id AND scope_p.access = 'workspace')))
RETURNING *;

-- name: ClearSquadProjects :many
-- Removes the squad from every project it is assigned to that the viewer sees.
UPDATE project SET squad_ids = array_remove(squad_ids, sqlc.arg('squad_id')::uuid), updated_at = now()
WHERE project.workspace_id = sqlc.arg('workspace_id') AND sqlc.arg('squad_id')::uuid = ANY(project.squad_ids)
-- projectaccess:scope
  AND (sqlc.arg(scope_mode)::text = 'all'
      OR EXISTS (SELECT 1 FROM project_member scope_pm
                 WHERE scope_pm.project_id = project.id AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
      OR (sqlc.arg(scope_mode)::text = 'member'
          AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = project.id AND scope_p.access = 'workspace')))
RETURNING *;

-- name: ListProjectsForSquad :many
-- The projects a squad is assigned to. A squad's active workflow changes what
-- its room shows for each of them, so their pages are announced.
SELECT * FROM project
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND sqlc.arg('squad_id')::uuid = ANY(squad_ids)
-- projectaccess:scope
AND (sqlc.arg(scope_mode)::text = 'all'
    OR (project.id IS NULL AND sqlc.arg(scope_mode)::text <> 'guest')
    OR EXISTS (SELECT 1 FROM project_member scope_pm
               WHERE scope_pm.project_id = project.id AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
    OR (sqlc.arg(scope_mode)::text = 'member'
        AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = project.id AND scope_p.access = 'workspace')))
ORDER BY created_at, id;
