-- name: InsertShadowConfig :one
-- A new shadow run on a seat (durable spec 9.4). The service has checked the workspace,
-- the seat, the candidate runtime and that the seat has no running run.
INSERT INTO shadow_config (
    id, workspace_id, agent_id, candidate_runtime_id, candidate_model, purpose,
    necessity_test_id, work_class_prefix, sample_rate, target_pairs, auto_promote, created_by
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(candidate_runtime_id),
    sqlc.arg(candidate_model), sqlc.arg(purpose), sqlc.narg('necessity_test_id'),
    sqlc.arg(work_class_prefix), sqlc.arg(sample_rate), sqlc.arg(target_pairs),
    sqlc.arg(auto_promote), sqlc.arg(created_by)
)
RETURNING *;

-- name: GetShadowConfig :one
SELECT * FROM shadow_config
WHERE id = @id AND workspace_id = @workspace_id;

-- name: LockShadowConfig :one
-- The config row is locked while the primary's claim decides whether to shadow it, so the
-- shadow agent is created once.
SELECT * FROM shadow_config
WHERE id = @id AND workspace_id = @workspace_id
FOR UPDATE;

-- name: GetRunningShadowConfigForAgent :one
-- The seat's running shadow run, if any. A seat has at most one at a time.
SELECT * FROM shadow_config
WHERE workspace_id = @workspace_id AND agent_id = @agent_id AND state = 'running'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: SetShadowConfigShadowAgent :execrows
UPDATE shadow_config
SET shadow_agent_id = @shadow_agent_id
WHERE id = @id AND shadow_agent_id IS NULL;

-- name: SetShadowConfigState :one
UPDATE shadow_config
SET state = @state,
    completed_at = CASE WHEN @state = 'running' THEN NULL ELSE now() END
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CreateShadowAgent :one
-- The shadow agent (D27): the seat's own role, instructions and configuration, on the
-- candidate runtime and model. Its name is given by the caller, because agent names are
-- unique in a workspace and the seat already has one. It is linked to the seat by
-- shadow_of_agent_id. It keeps no MCP overlay and no environment secrets, so it holds no
-- write token (spec 9.4 (b)).
INSERT INTO agent (
    workspace_id, name, description, runtime_mode, runtime_config, runtime_id,
    visibility, permission_mode, max_concurrent_tasks, owner_id, instructions,
    custom_env, custom_args, model, shadow_of_agent_id
)
SELECT
    a.workspace_id, sqlc.arg(name), a.description, a.runtime_mode, a.runtime_config, sqlc.arg(runtime_id),
    'private', 'private', a.max_concurrent_tasks, a.owner_id, a.instructions,
    '{}'::jsonb, '[]'::jsonb, sqlc.arg(model), a.id
FROM agent a
WHERE a.id = sqlc.arg(seat_id) AND a.workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: InsertShadowPair :one
-- One pair per primary task: a second claim of the same task inserts nothing (no row).
INSERT INTO shadow_pair (id, workspace_id, config_id, issue_id, primary_task_id)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(config_id), sqlc.arg(issue_id), sqlc.arg(primary_task_id))
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetShadowPairForPrimary :one
SELECT * FROM shadow_pair
WHERE primary_task_id = @primary_task_id;

-- name: SetShadowPairShadowTask :execrows
UPDATE shadow_pair
SET shadow_task_id = @shadow_task_id
WHERE id = @id AND shadow_task_id IS NULL;

-- name: SetShadowPairComparison :execrows
-- A verdict is written once: a pair already compared keeps its first comparison.
UPDATE shadow_pair
SET comparison = @comparison, comparison_reason = @comparison_reason, compared_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND comparison IS NULL;

-- name: GetShadowPair :one
SELECT * FROM shadow_pair
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListShadowPairsForConfig :many
SELECT * FROM shadow_pair
WHERE workspace_id = @workspace_id AND config_id = @config_id
ORDER BY created_at, id;

-- name: SetShadowTaskContext :execrows
-- Marks a shadow task with its run and pair, while the task is not yet claimed. The task
-- is then in the shadow run class, and budgets hold it.
UPDATE agent_task_queue
SET context = COALESCE(context, '{}'::jsonb)
    || jsonb_build_object(
        'durable_kind', 'shadow',
        'shadow_config_id', sqlc.arg(shadow_config_id)::text,
        'shadow_pair_id', sqlc.arg(shadow_pair_id)::text)
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'dispatched', 'deferred');
