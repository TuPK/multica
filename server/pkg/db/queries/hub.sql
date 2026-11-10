-- Skill / plugin hub: sources and the items they produced. Every query filters
-- by workspace_id. There are no foreign keys: source removal and item detach or
-- delete are application-owned, in a transaction where atomicity matters.

-- name: ListHubSourcesWithItemCount :many
SELECT
    hs.*,
    (SELECT count(*) FROM hub_item hi
        WHERE hi.workspace_id = hs.workspace_id AND hi.source_id = hs.id)::bigint AS item_count
FROM hub_source hs
WHERE hs.workspace_id = $1
ORDER BY hs.created_at ASC, hs.id ASC;

-- name: GetHubSourceInWorkspace :one
SELECT * FROM hub_source
WHERE id = $1 AND workspace_id = $2;

-- name: CreateHubSource :one
INSERT INTO hub_source (
    workspace_id, name, type, url, config, enabled, auto_sync_interval_sec, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: UpsertHubSourceByTypeURL :one
-- The import commit's entry point: an existing source for the same type and URL
-- is refreshed in place, so the caller never creates a duplicate.
INSERT INTO hub_source (
    workspace_id, name, type, url, config, enabled, auto_sync_interval_sec, created_by
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('name'), sqlc.arg('type'), sqlc.arg('url'),
    sqlc.arg('config'), sqlc.arg('enabled'), sqlc.arg('auto_sync_interval_sec'), sqlc.narg('created_by')
)
ON CONFLICT (workspace_id, type, url) DO UPDATE SET
    name = EXCLUDED.name,
    config = EXCLUDED.config,
    enabled = EXCLUDED.enabled,
    auto_sync_interval_sec = EXCLUDED.auto_sync_interval_sec,
    updated_at = now()
RETURNING *;

-- name: UpdateHubSource :one
UPDATE hub_source SET
    name = $3,
    url = $4,
    config = $5,
    enabled = $6,
    auto_sync_interval_sec = $7,
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeleteHubSource :exec
DELETE FROM hub_source WHERE id = $1 AND workspace_id = $2;

-- name: CountHubItemsBySource :one
SELECT count(*) FROM hub_item
WHERE workspace_id = $1 AND source_id = $2;

-- name: ListHubItemsBySource :many
SELECT id, kind, ref_id FROM hub_item
WHERE workspace_id = $1 AND source_id = $2
ORDER BY kind ASC, ref_id ASC;

-- name: DetachHubItemsBySource :exec
-- Keeps the items; they become manual entries with no source.
UPDATE hub_item SET
    source_id = NULL,
    tracking = 'manual',
    updated_at = now()
WHERE workspace_id = $1 AND source_id = $2;

-- name: DeleteHubItem :exec
DELETE FROM hub_item WHERE id = $1 AND workspace_id = $2;

-- name: DeleteHubItemsBySource :exec
DELETE FROM hub_item WHERE hub_item.workspace_id = $1 AND source_id = $2;

-- name: UpsertHubItem :one
INSERT INTO hub_item (
    workspace_id, kind, ref_id, source_id, source_ref, source_digest, tracking,
    latest_version, bundle_key, bundle_digest, size_bytes, file_count,
    scan_status, scan_report
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('kind'), sqlc.arg('ref_id'), sqlc.narg('source_id'),
    sqlc.arg('source_ref'), sqlc.arg('source_digest'), sqlc.arg('tracking'),
    sqlc.arg('latest_version'), sqlc.arg('bundle_key'), sqlc.arg('bundle_digest'),
    sqlc.arg('size_bytes'), sqlc.arg('file_count'), sqlc.arg('scan_status'), sqlc.arg('scan_report')
)
ON CONFLICT (workspace_id, kind, ref_id) DO UPDATE SET
    source_id = EXCLUDED.source_id,
    source_ref = EXCLUDED.source_ref,
    source_digest = EXCLUDED.source_digest,
    tracking = EXCLUDED.tracking,
    latest_version = EXCLUDED.latest_version,
    bundle_key = EXCLUDED.bundle_key,
    bundle_digest = EXCLUDED.bundle_digest,
    size_bytes = EXCLUDED.size_bytes,
    file_count = EXCLUDED.file_count,
    scan_status = EXCLUDED.scan_status,
    scan_report = EXCLUDED.scan_report,
    updated_at = now()
RETURNING *;

-- name: GetHubItemByRef :one
SELECT * FROM hub_item
WHERE workspace_id = $1 AND kind = $2 AND ref_id = $3;

-- name: ListHubItemsByWorkspace :many
SELECT * FROM hub_item
WHERE workspace_id = $1
ORDER BY kind ASC, created_at ASC, id ASC;

-- name: ListSkillsInUseByIDs :many
-- Of the given skills, the ones an agent or a squad still uses. Items in this
-- list block a source removal that would delete them.
SELECT s.id, s.name FROM skill s
WHERE s.workspace_id = $1
  AND s.id = ANY(sqlc.arg('skill_ids')::uuid[])
  AND (
      EXISTS (SELECT 1 FROM agent_skill ask WHERE ask.skill_id = s.id)
      OR EXISTS (SELECT 1 FROM squad_skill ss WHERE ss.skill_id = s.id AND ss.workspace_id = s.workspace_id)
  )
ORDER BY s.name ASC;

-- name: ListPluginPackagesInUseByIDs :many
-- Of the given plugin packages, the ones with an installed version somewhere in
-- the workspace. Installations are what the plugin delete path refuses on.
SELECT p.id, p.name FROM plugin_package p
WHERE p.workspace_id = $1
  AND p.id = ANY(sqlc.arg('package_ids')::uuid[])
  AND EXISTS (
      SELECT 1 FROM plugin_package_version v
      JOIN plugin_installation pi ON pi.package_version_id = v.id
      WHERE v.package_id = p.id
  )
ORDER BY p.name ASC;

-- name: DeleteWorkspaceHubData :exec
-- Workspace teardown: the Hub tables carry no foreign keys, so they are removed
-- explicitly. Bundles in object storage are removed by the handler after commit.
WITH
deleted_activity AS (
    DELETE FROM hub_activity WHERE hub_activity.workspace_id = $1
),
deleted_blobs AS (
    DELETE FROM hub_item_version_blob WHERE hub_item_version_blob.workspace_id = $1
),
deleted_items AS (
    DELETE FROM hub_item WHERE hub_item.workspace_id = $1
)
DELETE FROM hub_source WHERE hub_source.workspace_id = $1;

-- name: DeleteWorkspaceKnowledgePropertyHistory :exec
-- No foreign keys on this table either (migration 925), so teardown removes it.
DELETE FROM knowledge_property_history WHERE knowledge_property_history.workspace_id = $1;
