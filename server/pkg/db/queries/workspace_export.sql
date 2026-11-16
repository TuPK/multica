-- The export job and the export routes. Every read for the archive lists its
-- columns: a column that is not selected here can never reach the archive, and
-- the allowlist in internal/workspaceexport names the same set. The paged reads
-- take the id of the last row seen, so a workspace of any size is read in
-- bounded steps.

-- name: GetLatestWorkspaceExport :one
SELECT status, created_at
FROM workspace_export
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateWorkspaceExport :one
-- The unique partial index on (workspace_id) for queued and running rows makes
-- a second concurrent insert fail with a unique violation.
INSERT INTO workspace_export (id, workspace_id, requested_by, status)
VALUES (@id, @workspace_id, @requested_by, 'queued')
RETURNING *;

-- name: ListWorkspaceExports :many
SELECT *
FROM workspace_export
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT 100;

-- name: GetWorkspaceExport :one
SELECT *
FROM workspace_export
WHERE id = @id
  AND workspace_id = @workspace_id;

-- name: ClaimNextWorkspaceExport :one
-- Moves the oldest queued export to running. The status condition in the outer
-- update makes a second claim of the same row do nothing.
UPDATE workspace_export
SET status = 'running',
    started_at = @now
WHERE id = (
    SELECT q.id
    FROM workspace_export q
    WHERE q.status = 'queued'
    ORDER BY q.created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
  AND status = 'queued'
RETURNING *;

-- name: CompleteWorkspaceExport :one
-- Returns no row when the export is not running. The row gives the caller the
-- workspace for the audit row it writes in the same transaction.
UPDATE workspace_export
SET status = 'done',
    storage_key = @storage_key,
    size_bytes = @size_bytes,
    finished_at = @finished_at,
    expires_at = @expires_at
WHERE id = @id
  AND status = 'running'
RETURNING *;

-- name: FailWorkspaceExport :one
-- Returns no row when the export is not running.
UPDATE workspace_export
SET status = 'failed',
    error = @error,
    finished_at = @finished_at
WHERE id = @id
  AND status = 'running'
RETURNING *;

-- name: FailStaleWorkspaceExports :execrows
-- A running export whose job died would hold the one-at-a-time slot of its
-- workspace for good.
UPDATE workspace_export
SET status = 'failed',
    error = @error,
    finished_at = @finished_at
WHERE status = 'running'
  AND started_at < @started_before;

-- name: ListExpiredWorkspaceExports :many
SELECT *
FROM workspace_export
WHERE status = 'done'
  AND expires_at <= @now
ORDER BY expires_at
LIMIT @row_limit;

-- name: ExpireWorkspaceExport :execrows
UPDATE workspace_export
SET status = 'expired'
WHERE id = @id
  AND status = 'done';

-- name: GetWorkspaceForExport :one
-- settings and repos are left out: they are integration configuration, not
-- workspace content.
SELECT id, name, slug, description, context, issue_prefix, avatar_url, created_at, updated_at
FROM workspace
WHERE id = @workspace_id;

-- name: ListMembersForExport :many
SELECT m.id, m.workspace_id, m.user_id, m.role, m.created_at, u.name, u.email
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = @workspace_id
  AND m.id > @after
ORDER BY m.id
LIMIT @row_limit;

-- name: ListProjectsForExport :many
SELECT id, workspace_id, title, description, icon, status, priority, lead_type, lead_id,
       start_date, due_date, creator_type, creator_id, created_at, updated_at
FROM project
WHERE workspace_id = @workspace_id
  AND id > @after
ORDER BY id
LIMIT @row_limit;

-- name: ListIssuesForExport :many
-- The two jsonb columns are cast to text; the export parses them back, so the
-- archive holds JSON and not base64.
SELECT id, workspace_id, number, title, description, status, priority,
       assignee_type, assignee_id, creator_type, creator_id, parent_issue_id, project_id,
       start_date, due_date, done_at, acceptance_criteria::text AS acceptance_criteria,
       properties::text AS properties, created_at, updated_at
FROM issue
WHERE workspace_id = @workspace_id
  AND id > @after
ORDER BY id
LIMIT @row_limit;

-- name: ListCommentsForExport :many
SELECT id, workspace_id, issue_id, parent_id, author_type, author_id, content, type,
       resolved_at, created_at, updated_at
FROM comment
WHERE workspace_id = @workspace_id
  AND deleted_at IS NULL
  AND id > @after
ORDER BY id
LIMIT @row_limit;

-- name: ListLabelsForExport :many
SELECT id, workspace_id, name, color, description, resource_type, created_at, updated_at
FROM issue_label
WHERE workspace_id = @workspace_id
  AND id > @after
ORDER BY id
LIMIT @row_limit;

-- name: ListStatusesForExport :many
SELECT id, workspace_id, key, name, description, category, color, icon, is_system, position,
       archived_at, created_at, updated_at
FROM issue_status
WHERE workspace_id = @workspace_id
  AND id > @after
ORDER BY id
LIMIT @row_limit;

-- name: ListAttachmentsForExport :many
-- Metadata only: the stored url and the file bytes are not exported.
SELECT id, workspace_id, issue_id, comment_id, filename AS name, content_type, size_bytes, created_at
FROM attachment
WHERE workspace_id = @workspace_id
  AND id > @after
ORDER BY id
LIMIT @row_limit;
