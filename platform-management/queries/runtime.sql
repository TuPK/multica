-- Runtimes of the Delegora daemons, read for the staff console (spec R6 section
-- 3b.5, QT5). The platform reads agent_runtime and agent_task_queue and writes
-- only platform_runtime (the disable switch, migration 1031).

-- name: SearchRuntimes :many
-- The runtimes of the staff list, newest first. A scoped caller (admin) reads
-- the runtimes of its tenant's workspaces only.
SELECT
    r.id,
    r.name,
    r.workspace_id,
    COALESCE(tw.tenant_id::text, '')::text AS tenant_id,
    r.status,
    r.last_seen_at,
    r.device_info,
    COALESCE(r.metadata->>'cli_version', '')::text AS daemon_version,
    COALESCE(r.metadata->>'version', '')::text AS agent_version,
    r.owner_id,
    (pr.runtime_id IS NOT NULL)::boolean AS disabled,
    (SELECT count(*) FROM agent_task_queue q WHERE q.runtime_id = r.id AND q.status = 'running')::bigint AS running_tasks,
    r.created_at
FROM agent_runtime r
LEFT JOIN tenant_workspace tw ON tw.workspace_id = r.workspace_id
LEFT JOIN platform_runtime pr ON pr.runtime_id = r.id
WHERE (
        sqlc.narg('pattern')::text IS NULL
        OR r.name ILIKE sqlc.narg('pattern')::text
    )
  AND (sqlc.arg('status')::text = '' OR r.status = sqlc.arg('status')::text)
  AND (NOT sqlc.arg('scoped')::boolean OR tw.tenant_id = sqlc.arg('scope_tenant')::uuid)
  AND (sqlc.narg('tenant_filter')::uuid IS NULL OR tw.tenant_id = sqlc.narg('tenant_filter')::uuid)
  AND (sqlc.narg('workspace_filter')::uuid IS NULL OR r.workspace_id = sqlc.narg('workspace_filter')::uuid)
  AND (NOT sqlc.arg('disabled_only')::boolean OR pr.runtime_id IS NOT NULL)
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
        OR (r.created_at, r.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
    )
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetRuntimeForAdmin :one
-- One runtime with its workspace's tenant and disable state; no row when unknown.
SELECT
    r.id,
    r.name,
    r.workspace_id,
    COALESCE(tw.tenant_id::text, '')::text AS tenant_id,
    r.status,
    r.last_seen_at,
    r.device_info,
    COALESCE(r.metadata->>'cli_version', '')::text AS daemon_version,
    COALESCE(r.metadata->>'version', '')::text AS agent_version,
    r.owner_id,
    (pr.runtime_id IS NOT NULL)::boolean AS disabled,
    (SELECT count(*) FROM agent_task_queue q WHERE q.runtime_id = r.id AND q.status = 'running')::bigint AS running_tasks,
    r.created_at
FROM agent_runtime r
LEFT JOIN tenant_workspace tw ON tw.workspace_id = r.workspace_id
LEFT JOIN platform_runtime pr ON pr.runtime_id = r.id
WHERE r.id = $1;

-- name: ListRuntimeRecentTasks :many
-- The newest tasks of a runtime: state, times and error code only, never the
-- content (spec R6 section 3b.5, rule 3).
SELECT id, status, created_at, completed_at, error
FROM agent_task_queue
WHERE runtime_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- name: CountRuntimeFailedTasks :one
-- Failed tasks of a runtime in the last seven days.
SELECT count(*)::bigint FROM agent_task_queue
WHERE runtime_id = $1 AND status = 'failed' AND created_at >= now() - interval '7 days';

-- name: DisablePlatformRuntime :exec
-- Writes the disabled row, or keeps the one there (spec R6 section 3b.5, rule 4).
INSERT INTO platform_runtime (runtime_id, status, reason, changed_by, changed_at)
VALUES ($1, 'disabled', $2, $3, now())
ON CONFLICT (runtime_id) DO UPDATE
SET reason = EXCLUDED.reason, changed_by = EXCLUDED.changed_by, changed_at = now();

-- name: EnablePlatformRuntime :execrows
-- Deletes the disabled row. Enabling a runtime that is enabled changes nothing.
DELETE FROM platform_runtime WHERE runtime_id = $1;

-- name: PlatformRuntimeDisabled :one
SELECT EXISTS (SELECT 1 FROM platform_runtime WHERE runtime_id = $1);

-- name: CountRuntimesByStatus :many
-- The runtimes of every workspace in each status (online or offline). A status
-- with no runtime is absent.
SELECT r.status, count(*)::bigint AS total
FROM agent_runtime r
GROUP BY r.status;
