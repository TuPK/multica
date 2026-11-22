-- Issue workflows (MUL-7420). A workflow selects and orders statuses from
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

-- name: CreateFlowWorkflowVersion :one
-- A published version of a flow workflow (spec 8.1): locked from the moment it exists
-- (published_at), one of a lineage (lineage_id, version), never auto (flow_auto false)
-- and never a catalog row (no template_ref).
INSERT INTO issue_workflow (
    workspace_id, name, description, initial_status_key, steps, owner_id, squad_id, flow_config,
    lineage_id, version, published_at, published_by
) VALUES (
    sqlc.arg('workspace_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('description')::text,
    sqlc.arg('initial_status_key')::text,
    sqlc.arg('steps')::jsonb,
    sqlc.arg('owner_id')::uuid,
    sqlc.arg('squad_id')::uuid,
    sqlc.arg('flow_config')::jsonb,
    sqlc.arg('lineage_id')::uuid,
    sqlc.arg('version')::int,
    now(),
    sqlc.arg('published_by')::uuid
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
-- MUL-8472: private projects the caller cannot see.
AND (sqlc.narg('hidden_project_ids')::uuid[] IS NULL OR NOT (COALESCE(i.project_id, '00000000-0000-0000-0000-000000000000'::uuid) = ANY(sqlc.narg('hidden_project_ids')::uuid[])))
GROUP BY i.status
ORDER BY i.status;

-- name: RemapSquadIssueStatus :many
-- Moves a squad's routed issues (the same set CountSquadIssuesByStatus counts)
-- from one status to another.
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
-- MUL-8472: private projects the caller cannot see.
AND (sqlc.narg('hidden_project_ids')::uuid[] IS NULL OR NOT (COALESCE(i.project_id, '00000000-0000-0000-0000-000000000000'::uuid) = ANY(sqlc.narg('hidden_project_ids')::uuid[])))
RETURNING i.*;
