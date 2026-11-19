-- Workspace exports requested by staff (spec R6 section 3b.2, rule 3). The
-- platform inserts a queued row; Multica's workspace_export job runs it. The
-- unique index of migration 687 allows one queued or running export per workspace.

-- name: WorkspaceExportInFlight :one
SELECT EXISTS (
    SELECT 1 FROM workspace_export
    WHERE workspace_id = $1 AND status IN ('queued', 'running')
);

-- name: InsertWorkspaceExport :one
INSERT INTO workspace_export (id, workspace_id, requested_by, status)
VALUES (gen_random_uuid(), $1, $2, 'queued')
RETURNING id;
