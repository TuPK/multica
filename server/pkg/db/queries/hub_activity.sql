-- Skill / plugin hub: the activity feed, usage counts for quotas, and the raw
-- files of plugin versions for export. Every query filters by workspace_id.

-- name: InsertHubActivity :exec
INSERT INTO hub_activity (
    workspace_id, actor_type, actor_id, action, kind, ref_id, item_name, source_id, detail
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('actor_type'), sqlc.narg('actor_id'), sqlc.arg('action'),
    sqlc.narg('kind'), sqlc.narg('ref_id'), sqlc.arg('item_name'), sqlc.narg('source_id'), sqlc.arg('detail')
);

-- name: ListHubActivity :many
-- One page of the feed, newest first, keyset-paginated on (created_at, id). The
-- actor's display name comes from the user or agent table in the same query, so
-- a page costs one round trip. A departed member still resolves: the user table
-- is global, not membership-gated.
SELECT
    a.id,
    a.actor_type,
    a.actor_id,
    a.action,
    a.kind,
    a.ref_id,
    a.item_name,
    a.source_id,
    a.detail,
    a.created_at,
    (CASE
        WHEN a.actor_type = 'member' THEN COALESCE(u.name, '')
        WHEN a.actor_type = 'agent' THEN COALESCE(ag.name, '')
        ELSE ''
    END)::text AS actor_name
FROM hub_activity a
LEFT JOIN "user" u ON a.actor_type = 'member' AND u.id = a.actor_id
LEFT JOIN agent ag ON a.actor_type = 'agent' AND ag.id = a.actor_id
WHERE a.workspace_id = sqlc.arg('workspace_id')
  AND (sqlc.narg('kind')::text IS NULL OR a.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('ref_id')::uuid IS NULL OR a.ref_id = sqlc.narg('ref_id')::uuid)
  AND (sqlc.narg('source_id')::uuid IS NULL OR a.source_id = sqlc.narg('source_id')::uuid)
  AND (
      sqlc.narg('before_created_at')::timestamptz IS NULL
      OR (a.created_at, a.id) < (sqlc.narg('before_created_at')::timestamptz, sqlc.narg('before_id')::uuid)
  )
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg('row_limit');

-- name: CountHubUsage :one
-- What a workspace's quotas measure: its hub items and their bundle bytes.
SELECT
    count(*)::bigint AS item_count,
    COALESCE(sum(size_bytes), 0)::bigint AS storage_bytes
FROM hub_item
WHERE workspace_id = $1;

-- name: ListPluginPackageFileContents :many
-- Every stored file of one plugin version, with its bytes. The virtual manifest
-- is not a row; the export adds it through the plugin service.
SELECT path, content FROM plugin_package_file
WHERE version_id = $1
ORDER BY path ASC;
