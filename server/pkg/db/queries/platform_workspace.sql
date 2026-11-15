-- name: WorkspaceState :one
-- The platform writes platform_workspace; the server only reads it. A missing
-- row means the workspace is active.
SELECT status, deletion_scheduled_at
FROM platform_workspace
WHERE workspace_id = $1;
