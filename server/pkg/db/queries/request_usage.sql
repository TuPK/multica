-- name: GetRequestUsageSummary :one
-- Runs and tokens spent on a request issue and its direct children. The scope
-- is one level deep, matching the Secretary's request shape; a child's own
-- children are not summed here. Only issues the viewer sees are summed.
WITH scope AS (
    SELECT i.id
    FROM issue i
    WHERE i.workspace_id = sqlc.arg('workspace_id')::uuid
      AND (i.id = sqlc.arg('id')::uuid OR i.parent_issue_id = sqlc.arg('id')::uuid)
      -- MUL-8472: private projects the caller cannot see.
      AND (sqlc.narg('hidden_project_ids')::uuid[] IS NULL OR NOT (COALESCE(i.project_id, '00000000-0000-0000-0000-000000000000'::uuid) = ANY(sqlc.narg('hidden_project_ids')::uuid[])))
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
