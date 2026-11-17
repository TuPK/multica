-- The nightly usage snapshot (platform spec 5.13). One query per count, so each
-- count is defined next to the rule that bounds it. A count that is a snapshot
-- of the end of the day takes day_end; a count of what happened in the day takes
-- both bounds. The day is the half-open window [day_start, day_end) in UTC.

-- name: ListUsageWorkspaceIDs :many
-- Keyset page over every workspace, in id order.
SELECT id FROM workspace
WHERE id > sqlc.arg('after_id')
ORDER BY id
LIMIT sqlc.arg('row_limit');

-- name: CountUsageMembers :one
-- Members of the workspace at any point in the day [day_start, day_end): current
-- rows created before day_end, plus users removed during the day (their row is
-- deleted) who have no such row, so a remove-and-re-add counts once.
SELECT (
  (SELECT count(*) FROM member m
   WHERE m.workspace_id = sqlc.arg('workspace_id')
     AND m.created_at < sqlc.arg('day_end')::timestamptz)
  +
  (SELECT count(DISTINCT r.user_id) FROM member_removal r
   WHERE r.workspace_id = sqlc.arg('workspace_id')
     AND r.removed_at >= sqlc.arg('day_start')::timestamptz
     AND r.removed_at < sqlc.arg('day_end')::timestamptz
     AND NOT EXISTS (
       SELECT 1 FROM member m2
       WHERE m2.workspace_id = r.workspace_id
         AND m2.user_id = r.user_id
         AND m2.created_at < sqlc.arg('day_end')::timestamptz))
)::int;

-- name: CountUsageAgents :one
-- Agents that exist and are not archived at the end of the day.
SELECT count(*)::int FROM agent
WHERE workspace_id = sqlc.arg('workspace_id')
  AND created_at < sqlc.arg('day_end')::timestamptz
  AND (archived_at IS NULL OR archived_at >= sqlc.arg('day_end')::timestamptz);

-- name: CountUsageRuntimes :one
SELECT count(*)::int FROM agent_runtime
WHERE workspace_id = sqlc.arg('workspace_id')
  AND created_at < sqlc.arg('day_end')::timestamptz;

-- name: CountUsageIssuesTotal :one
-- Issues are deleted for good, so the issues that exist at the end of the day
-- are the ones created before it that still have a row.
SELECT count(*)::int FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')
  AND created_at < sqlc.arg('day_end')::timestamptz;

-- name: CountUsageIssuesCreated :one
SELECT count(*)::int FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')
  AND created_at >= sqlc.arg('day_start')::timestamptz
  AND created_at < sqlc.arg('day_end')::timestamptz;

-- name: CountUsageTasksRun :one
-- agent_task_queue has no workspace_id; a task belongs to its agent's workspace.
SELECT count(*)::int FROM agent_task_queue t
JOIN agent a ON a.id = t.agent_id
WHERE a.workspace_id = sqlc.arg('workspace_id')
  AND t.status = 'completed'
  AND t.completed_at >= sqlc.arg('day_start')::timestamptz
  AND t.completed_at < sqlc.arg('day_end')::timestamptz;

-- name: SumUsageStorageBytes :one
SELECT COALESCE(sum(size_bytes), 0)::bigint FROM attachment
WHERE workspace_id = sqlc.arg('workspace_id')
  AND created_at < sqlc.arg('day_end')::timestamptz;

-- name: UpsertPlatformUsageDaily :exec
-- A rerun for the same day replaces the row: the numbers are a snapshot of the
-- day, so they are never added to what is there.
INSERT INTO platform_usage_daily (
    workspace_id, day, members, agents, runtimes, issues_total, issues_created, tasks_run, storage_bytes
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('day'), sqlc.arg('members'), sqlc.arg('agents'), sqlc.arg('runtimes'),
    sqlc.arg('issues_total'), sqlc.arg('issues_created'), sqlc.arg('tasks_run'), sqlc.arg('storage_bytes')
)
ON CONFLICT (workspace_id, day) DO UPDATE SET
    members = EXCLUDED.members,
    agents = EXCLUDED.agents,
    runtimes = EXCLUDED.runtimes,
    issues_total = EXCLUDED.issues_total,
    issues_created = EXCLUDED.issues_created,
    tasks_run = EXCLUDED.tasks_run,
    storage_bytes = EXCLUDED.storage_bytes,
    computed_at = now();
