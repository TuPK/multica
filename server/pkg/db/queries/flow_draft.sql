-- Drafts of a flow workflow (spec 2.2). Every query is scoped by workspace; the ones
-- a squad's routes use are scoped by squad too.

-- name: CreateFlowDraft :one
INSERT INTO issue_workflow_draft (
    workspace_id, squad_id, lineage_id, base_workflow_id, name, doc, origin, source_ref, created_by, updated_by
) VALUES (
    sqlc.arg('workspace_id')::uuid,
    sqlc.arg('squad_id')::uuid,
    sqlc.narg('lineage_id')::uuid,
    sqlc.narg('base_workflow_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('doc')::jsonb,
    sqlc.arg('origin')::text,
    sqlc.narg('source_ref')::jsonb,
    sqlc.arg('actor_id')::uuid,
    sqlc.arg('actor_id')::uuid
)
RETURNING *;

-- name: GetFlowDraft :one
SELECT * FROM issue_workflow_draft
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid;

-- name: ListFlowDrafts :many
-- A squad's drafts, newest first (idx_issue_workflow_draft_squad).
SELECT * FROM issue_workflow_draft
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid
ORDER BY updated_at DESC, id;

-- name: CountFlowDrafts :one
SELECT COUNT(*)::bigint FROM issue_workflow_draft
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid;

-- name: UpdateFlowDraft :one
-- Writes a draft the caller read at this revision; no row means someone else wrote first.
UPDATE issue_workflow_draft SET
    name = COALESCE(sqlc.narg('name')::text, name),
    doc = COALESCE(sqlc.narg('doc')::jsonb, doc),
    revision = revision + 1,
    updated_by = sqlc.arg('actor_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid
  AND revision = sqlc.arg('revision')::int
RETURNING *;

-- name: SetFlowDraftBase :one
-- After a publish the draft is kept, and its base is the version just made.
UPDATE issue_workflow_draft SET
    lineage_id = sqlc.arg('lineage_id')::uuid,
    base_workflow_id = sqlc.arg('base_workflow_id')::uuid,
    revision = revision + 1,
    updated_by = sqlc.arg('actor_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid
RETURNING *;

-- name: DeleteFlowDraft :execrows
DELETE FROM issue_workflow_draft
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND squad_id = sqlc.arg('squad_id')::uuid;

-- name: CountFlowLineageVersions :one
-- The versions a lineage has, for the cap.
SELECT COUNT(*)::bigint FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND lineage_id = sqlc.arg('lineage_id')::uuid;

-- name: LatestFlowLineageVersion :one
-- The newest version of a lineage; no row when the lineage has none.
SELECT * FROM issue_workflow
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND lineage_id = sqlc.arg('lineage_id')::uuid
ORDER BY version DESC
LIMIT 1;

-- name: DeleteFlowDraftsForWorkspace :exec
-- No foreign keys by project rule, so workspace teardown cleans up here.
DELETE FROM issue_workflow_draft WHERE workspace_id = sqlc.arg('workspace_id')::uuid;
