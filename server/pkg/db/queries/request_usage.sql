-- name: GetRequestUsageSummary :one
-- Runs and tokens spent on a request issue and its direct children. The scope
-- is one level deep, matching the Secretary's request shape; a child's own
-- children are not summed here. Only issues the viewer sees are summed.
WITH scope AS (
    SELECT i.id
    FROM issue i
    WHERE i.workspace_id = sqlc.arg('workspace_id')::uuid
      AND (i.id = sqlc.arg('id')::uuid OR i.parent_issue_id = sqlc.arg('id')::uuid)
      -- projectaccess:scope
      AND (sqlc.arg(scope_mode)::text = 'all'
          OR (i.project_id IS NULL AND sqlc.arg(scope_mode)::text <> 'guest')
          OR EXISTS (SELECT 1 FROM project_member scope_pm
                     WHERE scope_pm.project_id = i.project_id AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
          OR (sqlc.arg(scope_mode)::text = 'member'
              AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = i.project_id AND scope_p.access = 'workspace')))
)
SELECT
    COUNT(DISTINCT atq.id)::int AS run_count,
    COUNT(DISTINCT tu.task_id)::int AS metered_run_count,
    COALESCE(SUM(tu.input_tokens), 0)::bigint AS input_tokens,
    COALESCE(SUM(tu.output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(tu.cache_read_tokens), 0)::bigint AS cache_read_tokens,
    COALESCE(SUM(tu.cache_write_tokens), 0)::bigint AS cache_write_tokens,
    COALESCE(SUM(tu.cost_usd_ticks), 0)::bigint AS cost_usd_ticks
FROM scope s
JOIN agent_task_queue atq ON atq.issue_id = s.id
LEFT JOIN task_usage tu ON tu.task_id = atq.id;
