-- Project workflows (MUL-7420). A workflow selects and orders statuses from
-- the workspace's issue_status catalog. Projects opt in through
-- project.workflow_id; a project without one uses the workspace's Default
-- workflow row. Each step also stores who an issue entering it is handed to.

-- name: ListIssueWorkflows :many
SELECT * FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
ORDER BY is_default DESC, lower(name), created_at, id;

-- name: GetDefaultIssueWorkflow :one
SELECT * FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND is_default;

-- name: CreateDefaultIssueWorkflow :many
-- Returns no row when another request created the Default first.
INSERT INTO issue_workflow (workspace_id, name, description, initial_status_key, steps, is_default)
VALUES (
    sqlc.arg('workspace_id')::uuid,
    'Default',
    '',
    'todo',
    sqlc.arg('steps')::jsonb,
    true
)
ON CONFLICT (workspace_id) WHERE is_default DO NOTHING
RETURNING *;

-- name: ListNamedIssueWorkflowSteps :many
-- The steps of every workflow except Default, for the Default snapshot.
SELECT steps FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND NOT is_default;

-- name: RemoveStatusFromDefaultIssueWorkflow :exec
-- An archived status leaves Default with it; Default never blocks an archive.
UPDATE issue_workflow SET
    steps = COALESCE((
        SELECT jsonb_agg(step ORDER BY ord)
        FROM jsonb_array_elements(steps) WITH ORDINALITY AS s(step, ord)
        WHERE step->>'status_key' <> sqlc.arg('status_key')::text
    ), '[]'::jsonb),
    updated_at = now()
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND is_default
  AND steps @> jsonb_build_array(jsonb_build_object('status_key', sqlc.arg('status_key')::text));

-- name: ListProjectsUsingDefaultIssueWorkflow :many
SELECT * FROM project
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND workflow_id IS NULL
ORDER BY created_at, id;

-- name: GetIssueWorkflow :one
SELECT * FROM issue_workflow
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: CreateIssueWorkflow :one
-- flow_config and template_ref are set only for a workflow created from a catalog flow.
INSERT INTO issue_workflow (workspace_id, name, description, initial_status_key, steps, owner_id, squad_id, flow_config, template_ref)
VALUES (
    sqlc.arg('workspace_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('description')::text,
    sqlc.arg('initial_status_key')::text,
    sqlc.arg('steps')::jsonb,
    sqlc.arg('owner_id')::uuid,
    sqlc.narg('squad_id')::uuid,
    sqlc.narg('flow_config')::jsonb,
    sqlc.narg('template_ref')::text
)
RETURNING *;

-- name: ListSquadIssueWorkflows :many
-- A squad's library: the workflows it owns, in the order its picker lists them.
SELECT * FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid
ORDER BY lower(name), created_at, id;

-- name: SetIssueWorkflowSquad :one
-- Adopts a workspace-level workflow into a squad's library.
UPDATE issue_workflow SET
    squad_id = sqlc.arg('squad_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
RETURNING *;

-- name: UpdateIssueWorkflow :one
UPDATE issue_workflow SET
    name = sqlc.arg('name')::text,
    description = sqlc.arg('description')::text,
    initial_status_key = sqlc.arg('initial_status_key')::text,
    steps = sqlc.arg('steps')::jsonb,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
RETURNING *;

-- name: DeleteIssueWorkflow :execrows
DELETE FROM issue_workflow
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND NOT is_default;

-- name: DeleteIssueWorkflowsForWorkspace :exec
-- No foreign keys by project rule, so workspace teardown cleans up here.
DELETE FROM issue_workflow WHERE workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: ListIssueWorkflowNamesUsingStatusKey :many
-- Archive guard for the status catalog: a status a workflow still lists cannot
-- be retired, or that workflow's projects would lose a column.
SELECT name FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND NOT is_default
  AND steps @> jsonb_build_array(jsonb_build_object('status_key', sqlc.arg('status_key')::text))
ORDER BY lower(name);

-- name: ListProjectsUsingIssueWorkflow :many
SELECT * FROM project
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND workflow_id = sqlc.arg('workflow_id')::uuid
ORDER BY created_at, id;

-- name: CountProjectIssuesByStatus :many
-- Status census for a project workflow switch or step removal, including
-- terminal issues: every issue of the project that the project's workflow
-- governs must end on a status it lists. Issues a squad routes through its own
-- active workflow are left out (see the NOT EXISTS below).
SELECT i.status, COUNT(*)::bigint AS issue_count
FROM issue i
WHERE i.workspace_id = sqlc.arg('workspace_id')::uuid
  AND i.project_id = ANY(sqlc.arg('project_ids')::uuid[])
  -- A request a flow runs and its tasks follow the flow's steps, not this workflow's.
  AND i.flow IS NULL
  AND i.flow_task IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM squad s
      WHERE s.workspace_id = i.workspace_id
        AND s.active_workflow_id IS NOT NULL
        -- A squad on a flow routes only its flow requests; its other issues keep the project's workflow.
        AND NOT EXISTS (SELECT 1 FROM issue_workflow fw WHERE fw.id = s.active_workflow_id AND fw.flow_config IS NOT NULL)
        AND s.id = CASE
            WHEN i.assignee_type = 'squad' THEN i.assignee_id
            WHEN i.assignee_type = 'agent' THEN (SELECT a.home_squad_id FROM agent a WHERE a.id = i.assignee_id)
        END
  )
GROUP BY i.status
ORDER BY i.status;

-- name: RemapProjectIssueStatus :many
-- Moves every issue of the given projects that the project's workflow governs
-- from one status to another as part of an administrative workflow change.
-- Mirrors UpdateIssueStatus: the issue is re-ranked to the top of its new
-- column, the revision advances, and a duplicate mark only survives
-- cancelled -> cancelled (never the case here, because from and to differ). It
-- deliberately touches nothing else, so it never hands an issue off or starts a
-- run. The squad-routed issues CountProjectIssuesByStatus leaves out are left
-- out here too.
UPDATE issue AS i SET
    status = sqlc.arg('to_status')::text,
    duplicate_of_issue_id = NULL,
    position = (
        SELECT COALESCE(MIN(target.position), 0) - 1
        FROM issue AS target
        WHERE target.workspace_id = i.workspace_id
          AND target.status = sqlc.arg('to_status')::text
    ),
    revision = i.revision + 1,
    last_activity_at = GREATEST(COALESCE(i.last_activity_at, i.updated_at), now()),
    updated_at = now()
WHERE i.workspace_id = sqlc.arg('workspace_id')::uuid
  AND i.project_id = ANY(sqlc.arg('project_ids')::uuid[])
  AND i.status = sqlc.arg('from_status')::text
  AND i.status <> sqlc.arg('to_status')::text
  -- A request a flow runs and its tasks follow the flow's steps, not this workflow's.
  AND i.flow IS NULL
  AND i.flow_task IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM squad s
      WHERE s.workspace_id = i.workspace_id
        AND s.active_workflow_id IS NOT NULL
        -- A squad on a flow routes only its flow requests; its other issues keep the project's workflow.
        AND NOT EXISTS (SELECT 1 FROM issue_workflow fw WHERE fw.id = s.active_workflow_id AND fw.flow_config IS NOT NULL)
        AND s.id = CASE
            WHEN i.assignee_type = 'squad' THEN i.assignee_id
            WHEN i.assignee_type = 'agent' THEN (SELECT a.home_squad_id FROM agent a WHERE a.id = i.assignee_id)
        END
  )
RETURNING i.*;

-- name: CountSquadIssuesByStatus :many
-- Status census for a squad's active-workflow switch or an edit of that workflow:
-- the project issues routed to the squad, either assigned to it or to an agent
-- whose home squad it is. Issues outside any project take no workflow.
SELECT i.status, COUNT(*)::bigint AS issue_count
FROM issue i
WHERE i.workspace_id = sqlc.arg('workspace_id')::uuid
  AND i.project_id IS NOT NULL
  -- A request a flow runs and its tasks follow the flow's steps, not this workflow's.
  AND i.flow IS NULL
  AND i.flow_task IS NULL
  AND (
      (i.assignee_type = 'squad' AND i.assignee_id = sqlc.arg('squad_id')::uuid)
      OR (i.assignee_type = 'agent' AND i.assignee_id IN (
          SELECT a.id FROM agent a
          WHERE a.workspace_id = sqlc.arg('workspace_id')::uuid
            AND a.home_squad_id = sqlc.arg('squad_id')::uuid
      ))
  )
GROUP BY i.status
ORDER BY i.status;

-- name: RemapSquadIssueStatus :many
-- Moves a squad's routed project issues (the same set CountSquadIssuesByStatus
-- counts) from one status to another, with RemapProjectIssueStatus's effects.
UPDATE issue AS i SET
    status = sqlc.arg('to_status')::text,
    duplicate_of_issue_id = NULL,
    position = (
        SELECT COALESCE(MIN(target.position), 0) - 1
        FROM issue AS target
        WHERE target.workspace_id = i.workspace_id
          AND target.status = sqlc.arg('to_status')::text
    ),
    revision = i.revision + 1,
    last_activity_at = GREATEST(COALESCE(i.last_activity_at, i.updated_at), now()),
    updated_at = now()
WHERE i.workspace_id = sqlc.arg('workspace_id')::uuid
  AND i.project_id IS NOT NULL
  -- A request a flow runs and its tasks follow the flow's steps, not this workflow's.
  AND i.flow IS NULL
  AND i.flow_task IS NULL
  AND (
      (i.assignee_type = 'squad' AND i.assignee_id = sqlc.arg('squad_id')::uuid)
      OR (i.assignee_type = 'agent' AND i.assignee_id IN (
          SELECT a.id FROM agent a
          WHERE a.workspace_id = sqlc.arg('workspace_id')::uuid
            AND a.home_squad_id = sqlc.arg('squad_id')::uuid
      ))
  )
  AND i.status = sqlc.arg('from_status')::text
  AND i.status <> sqlc.arg('to_status')::text
RETURNING i.*;

-- name: LockProjectForWorkflowChange :one
-- Serializes concurrent workflow switches of one project.
SELECT * FROM project
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
FOR UPDATE;

-- name: SetProjectWorkflow :one
UPDATE project SET
    workflow_id = sqlc.narg('workflow_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
RETURNING *;
