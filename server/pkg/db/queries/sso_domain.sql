-- name: ListSSODomains :many
SELECT * FROM sso_domain WHERE workspace_id = $1 ORDER BY domain;

-- name: GetSSODomain :one
SELECT * FROM sso_domain WHERE workspace_id = $1 AND domain = $2;

-- name: ListVerifiedSSODomainNames :many
SELECT domain FROM sso_domain WHERE workspace_id = $1 AND verified_at IS NOT NULL;

-- name: GetVerifiedSSODomainOwner :one
SELECT workspace_id FROM sso_domain WHERE domain = $1 AND verified_at IS NOT NULL;

-- A second add of the same claim returns the stored row, so its token stays.
-- name: AddSSODomain :one
INSERT INTO sso_domain (workspace_id, domain, token)
VALUES ($1, $2, $3)
ON CONFLICT (workspace_id, domain) DO UPDATE SET domain = EXCLUDED.domain
RETURNING *;

-- name: MarkSSODomainVerified :one
UPDATE sso_domain SET verified_at = now()
WHERE workspace_id = $1 AND domain = $2 AND verified_at IS NULL
RETURNING *;

-- name: DeleteSSODomain :execrows
DELETE FROM sso_domain WHERE workspace_id = $1 AND domain = $2;

-- name: DeleteSSODomainsByWorkspace :exec
DELETE FROM sso_domain WHERE workspace_id = $1;
