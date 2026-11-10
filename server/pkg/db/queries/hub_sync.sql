-- Skill / plugin hub: source sync and item update state (Phase 5). Every query
-- filters by workspace_id, except the auto-sync sweep and the source lookup by id,
-- which the scheduler runs across workspaces; they return a source's own
-- workspace_id, and the caller works inside it.

-- name: GetHubSourceByID :one
SELECT * FROM hub_source WHERE id = $1;

-- name: ListDueHubSources :many
-- Enabled sources with an auto-sync interval whose last sync is at least that old
-- (or that never synced), oldest first. The Go predicate hubSourceIsDue mirrors it.
SELECT * FROM hub_source
WHERE enabled
  AND auto_sync_interval_sec > 0
  AND (last_sync_at IS NULL OR last_sync_at <= now() - make_interval(secs => auto_sync_interval_sec))
ORDER BY last_sync_at ASC NULLS FIRST, id ASC
LIMIT sqlc.arg('row_limit');

-- name: TryHubSourceSyncLock :one
-- A transaction-scoped advisory lock per source. Manual and automatic syncs of one
-- source take it, so two runs (on this or another instance) never overlap. It is
-- released when the holding transaction ends.
SELECT pg_try_advisory_xact_lock(hashtextextended('hub_source_sync:' || sqlc.arg('source_id')::text, 0)) AS locked;

-- name: UpdateHubSourceSyncState :exec
UPDATE hub_source SET
    last_sync_at = now(),
    last_sync_status = sqlc.arg('status'),
    last_sync_error = sqlc.arg('error')
WHERE id = sqlc.arg('id') AND workspace_id = sqlc.arg('workspace_id');

-- name: ListTrackedHubItemsBySource :many
-- The items a sync checks: those still tracking the source. Forked and manual items
-- are never refetched.
SELECT * FROM hub_item
WHERE workspace_id = $1 AND source_id = $2 AND tracking = 'tracking'
ORDER BY kind ASC, created_at ASC, id ASC;

-- name: UpdateHubItemSyncState :exec
-- The update-check result of one item. checked_at is the time of this check.
UPDATE hub_item SET
    update_available = sqlc.arg('update_available'),
    remote_digest = sqlc.arg('remote_digest'),
    local_modified = sqlc.arg('local_modified'),
    checked_at = now()
WHERE id = sqlc.arg('id') AND workspace_id = sqlc.arg('workspace_id');

-- name: MarkHubItemForked :execrows
-- Detaches a tracking item from its source. Only a tracking item changes; the
-- row count tells the caller whether it did.
UPDATE hub_item SET tracking = 'forked', updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND tracking = 'tracking';

-- name: MarkHubItemLocalModified :exec
-- A local edit to a tracked skill or plugin. Best effort: the next check recomputes
-- the flag, so a write that misses here only delays the notice.
UPDATE hub_item SET local_modified = true
WHERE workspace_id = sqlc.arg('workspace_id') AND kind = sqlc.arg('kind')
  AND ref_id = sqlc.arg('ref_id') AND tracking = 'tracking';
