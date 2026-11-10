-- Skill / plugin hub: bundle storage. Every query filters by workspace_id, or by an
-- id the caller has already checked against its workspace. There are no foreign keys.

-- name: InsertHubItemIfMissing :exec
-- Creates the hub row of an existing skill or plugin the first time it is listed or
-- served. An existing row is left as it is, so concurrent callers are harmless.
INSERT INTO hub_item (
    workspace_id, kind, ref_id, source_id, tracking, latest_version, size_bytes, file_count
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('kind'), sqlc.arg('ref_id'), sqlc.narg('source_id'),
    sqlc.arg('tracking'), sqlc.arg('latest_version'), sqlc.arg('size_bytes'), sqlc.arg('file_count')
)
ON CONFLICT (workspace_id, kind, ref_id) DO NOTHING;

-- name: GetHubSourceByTypeURL :one
SELECT * FROM hub_source
WHERE workspace_id = $1 AND type = $2 AND url = $3;

-- name: UpdateHubItemBundleIfUnset :one
-- Records the bundle of an item that has none yet. The guard makes the first writer
-- win when two requests build the bundle at once; the loser gets no row back.
UPDATE hub_item SET
    bundle_key = sqlc.arg('bundle_key'),
    bundle_digest = sqlc.arg('bundle_digest'),
    latest_version = sqlc.arg('latest_version'),
    size_bytes = sqlc.arg('size_bytes'),
    file_count = sqlc.arg('file_count'),
    updated_at = now()
WHERE id = sqlc.arg('id') AND workspace_id = sqlc.arg('workspace_id') AND bundle_key = ''
RETURNING *;

-- name: InsertHubItemVersionBlob :exec
-- Versions are immutable: a repeat of the same version and digest is a no-op.
INSERT INTO hub_item_version_blob (
    workspace_id, item_id, version, bundle_key, digest, size_bytes, created_by
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('item_id'), sqlc.arg('version'), sqlc.arg('bundle_key'),
    sqlc.arg('digest'), sqlc.arg('size_bytes'), sqlc.narg('created_by')
)
ON CONFLICT (item_id, version, digest) DO NOTHING;

-- name: ListSkillBundleStatsByIDs :many
-- The size and file count a skill's bundle would have. The bundle is SKILL.md (the
-- skill content) plus every supporting file, as hubSkillBundle builds it.
SELECT s.id AS skill_id,
       (1 + count(sf.id) FILTER (WHERE sf.path <> 'SKILL.md'))::bigint AS file_count,
       (octet_length(s.content)
           + coalesce(sum(octet_length(sf.content)) FILTER (WHERE sf.path <> 'SKILL.md'), 0))::bigint AS size_bytes
FROM skill s
LEFT JOIN skill_file sf ON sf.skill_id = s.id
WHERE s.workspace_id = sqlc.arg('workspace_id')
  AND s.id = ANY(sqlc.arg('skill_ids')::uuid[])
GROUP BY s.id, s.content;

-- name: ListLatestPluginPackageVersionsByIDs :many
-- The newest published version of each given plugin package. Its bundle is the
-- package files plus the manifest, which lives on the version row.
SELECT DISTINCT ON (v.package_id)
       v.id, v.package_id, v.version, v.manifest,
       (v.size_bytes + octet_length(v.manifest::text))::bigint AS size_bytes,
       (1 + (SELECT count(*) FROM plugin_package_file f WHERE f.version_id = v.id))::bigint AS file_count,
       v.created_at
FROM plugin_package_version v
WHERE v.workspace_id = sqlc.arg('workspace_id')
  AND v.package_id = ANY(sqlc.arg('package_ids')::uuid[])
ORDER BY v.package_id, v.created_at DESC, v.id DESC;

-- name: ListHubPluginVersionFiles :many
-- The files of one published plugin version, for building its bundle.
SELECT path, content FROM plugin_package_file
WHERE version_id = $1
ORDER BY path ASC;
