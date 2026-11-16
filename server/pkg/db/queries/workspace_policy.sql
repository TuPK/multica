-- name: GetWorkspacePolicy :one
SELECT * FROM workspace_policy WHERE workspace_id = $1;

-- name: GetWorkspacePolicyForUpdate :one
SELECT * FROM workspace_policy WHERE workspace_id = $1 FOR UPDATE;

-- name: UpsertWorkspacePolicy :one
INSERT INTO workspace_policy (
    workspace_id, allowed_invite_domains, share_links_enabled, require_sso,
    require_mfa, session_max_age_hours, updated_by, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (workspace_id) DO UPDATE SET
    allowed_invite_domains = EXCLUDED.allowed_invite_domains,
    share_links_enabled = EXCLUDED.share_links_enabled,
    require_sso = EXCLUDED.require_sso,
    require_mfa = EXCLUDED.require_mfa,
    session_max_age_hours = EXCLUDED.session_max_age_hours,
    updated_by = EXCLUDED.updated_by,
    updated_at = now()
RETURNING *;
