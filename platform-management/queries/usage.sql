-- Usage numbers for the console (platform spec 5.13). The platform reads the
-- table that Multica's nightly job writes; it never computes the daily numbers.
--
-- GetCurrentWorkspaceCounts reads the content tables to count them, and only to
-- count them: each subquery is a bare count or sum filtered by the workspace id.
-- A test in internal/usage keeps it that way. This file is outside the console
-- scan of isolation_test.go on purpose; the counts are the "numeric counts" that
-- I6 allows, and no row or column of content leaves the database.

-- name: ListLatestUsageDaily :many
-- The newest row of each listed workspace. A workspace with no row is absent.
-- The unique index on (workspace_id, day) serves the lookup.
SELECT DISTINCT ON (workspace_id)
    workspace_id, day, members, agents, runtimes, issues_total, issues_created, tasks_run, storage_bytes
FROM platform_usage_daily
WHERE workspace_id = ANY($1::uuid[])
ORDER BY workspace_id, day DESC;

-- name: SumLatestUsageDaily :one
-- The totals of each workspace's newest row among the days since $1. A workspace
-- whose newest row is older than that is counted as having none, so the scan
-- stays bounded as the table grows.
WITH latest AS (
    SELECT DISTINCT ON (workspace_id)
        workspace_id, day, members, agents, runtimes, issues_total, issues_created, tasks_run, storage_bytes
    FROM platform_usage_daily
    WHERE day >= $1::date
    ORDER BY workspace_id, day DESC
)
SELECT
    count(*)::bigint AS workspaces,
    max(day)::date AS day,
    COALESCE(sum(members), 0)::bigint AS members,
    COALESCE(sum(agents), 0)::bigint AS agents,
    COALESCE(sum(runtimes), 0)::bigint AS runtimes,
    COALESCE(sum(issues_total), 0)::bigint AS issues_total,
    COALESCE(sum(issues_created), 0)::bigint AS issues_created,
    COALESCE(sum(tasks_run), 0)::bigint AS tasks_run,
    COALESCE(sum(storage_bytes), 0)::bigint AS storage_bytes
FROM latest;

-- name: GetCurrentWorkspaceCounts :one
-- One count per number, each filtered by the workspace id. Nothing here scans
-- another workspace's rows.
SELECT
    (SELECT count(*) FROM member m WHERE m.workspace_id = $1)::bigint AS members,
    (SELECT count(*) FROM agent a WHERE a.workspace_id = $1 AND a.archived_at IS NULL)::bigint AS agents,
    (SELECT count(*) FROM agent_runtime r WHERE r.workspace_id = $1)::bigint AS runtimes,
    (SELECT count(*) FROM issue i WHERE i.workspace_id = $1)::bigint AS issues_total,
    (SELECT COALESCE(sum(t.size_bytes), 0) FROM attachment t WHERE t.workspace_id = $1)::bigint AS storage_bytes;
