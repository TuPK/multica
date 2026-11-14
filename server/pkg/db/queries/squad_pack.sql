-- What a pack of skills and agents installed (spec 16.3). Every query is scoped by workspace.

-- name: CreateSquadPackInstall :one
INSERT INTO squad_pack_install (
    id, workspace_id, squad_id, pack_id, pack_version, format, digest, source, roster, installed_by
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('workspace_id')::uuid,
    sqlc.narg('squad_id')::uuid,
    sqlc.arg('pack_id')::text,
    sqlc.arg('pack_version')::text,
    sqlc.arg('format')::text,
    sqlc.arg('digest')::text,
    sqlc.arg('source')::jsonb,
    sqlc.arg('roster')::jsonb,
    sqlc.arg('installed_by')::uuid
)
RETURNING *;

-- name: GetSquadPackInstallByDigest :one
-- The latest install of a bundle, whatever squad it made.
SELECT * FROM squad_pack_install
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND digest = sqlc.arg('digest')::text
ORDER BY installed_at DESC
LIMIT 1;

-- name: GetLatestSquadPackInstallByPackID :one
-- The latest install of a pack that says its own id, any version of it.
SELECT * FROM squad_pack_install
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND pack_id = sqlc.arg('pack_id')::text
ORDER BY installed_at DESC
LIMIT 1;

-- name: GetAgentByNameInWorkspace :one
-- Archived agents count: the name stays taken (agent_workspace_name_unique).
SELECT * FROM agent
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND name = sqlc.arg('name')::text;

-- name: DeleteSquadPackInstallsForWorkspace :exec
-- No foreign keys by project rule, so workspace teardown cleans up here.
DELETE FROM squad_pack_install WHERE workspace_id = sqlc.arg('workspace_id')::uuid;
