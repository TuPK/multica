-- name: GetSSOConnection :one
SELECT * FROM sso_connection WHERE id = $1;

-- name: GetSSOConnectionByWorkspace :one
SELECT * FROM sso_connection WHERE workspace_id = $1;

-- The connection that signs in an email domain: the domain must be verified
-- by the workspace that owns the connection.
-- name: GetActiveSSOConnectionByVerifiedDomain :one
SELECT c.* FROM sso_connection c
JOIN sso_domain d ON d.workspace_id = c.workspace_id
WHERE d.domain = $1 AND d.verified_at IS NOT NULL AND c.status = 'active';

-- name: HasVerifiedSSODomain :one
SELECT EXISTS (
    SELECT 1 FROM sso_domain
    WHERE workspace_id = $1 AND domain = $2 AND verified_at IS NOT NULL
);

-- Needs the unique index of migration 697 on workspace_id.
-- name: UpsertSSOConnection :one
INSERT INTO sso_connection (workspace_id, issuer, client_id, client_secret_enc, default_role, status, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (workspace_id) DO UPDATE SET
    issuer = EXCLUDED.issuer,
    client_id = EXCLUDED.client_id,
    client_secret_enc = EXCLUDED.client_secret_enc,
    default_role = EXCLUDED.default_role,
    status = EXCLUDED.status,
    updated_at = now()
RETURNING *;

-- name: DeleteSSOConnection :execrows
DELETE FROM sso_connection WHERE workspace_id = $1;
