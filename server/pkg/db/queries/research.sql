-- =====================
-- Research Lab source settings
-- =====================

-- name: GetResearchSettings :one
SELECT * FROM research_settings
WHERE workspace_id = $1;

-- name: UpsertResearchSettings :one
-- The caller reads the row, applies its change, and writes every column back.
INSERT INTO research_settings (workspace_id, openalex_key_sealed, s2_key_sealed, contact_email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id) DO UPDATE
SET openalex_key_sealed = EXCLUDED.openalex_key_sealed,
    s2_key_sealed = EXCLUDED.s2_key_sealed,
    contact_email = EXCLUDED.contact_email,
    updated_at = now()
RETURNING *;

-- name: DeleteWorkspaceResearchSettings :exec
-- Workspace teardown only: the settings go with their workspace.
DELETE FROM research_settings WHERE workspace_id = sqlc.arg('workspace_id')::uuid;
