-- name: InsertUserConsent :execrows
-- Accepting the same (document, version) twice records one row. The affected
-- row count tells the caller whether this acceptance was new.
INSERT INTO user_consent (user_id, document, version, ip)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, document, version) DO NOTHING;

-- name: ListUserConsents :many
SELECT user_id, document, version, accepted_at, ip FROM user_consent
WHERE user_id = $1
ORDER BY accepted_at, document, version;

-- name: ExportListUserMemberships :many
SELECT w.id AS workspace_id, w.name AS workspace_name, m.role, m.created_at AS joined_at
FROM member m
JOIN workspace w ON w.id = m.workspace_id
WHERE m.user_id = $1
ORDER BY m.created_at, w.id;

-- name: ExportListUserPersonalAccessTokens :many
-- Explicit columns: token_hash is never selected.
SELECT id, name, token_prefix, revoked, created_at, last_used_at, expires_at
FROM personal_access_token
WHERE user_id = $1
ORDER BY created_at, id;

-- name: ExportListUserComposioConnections :many
-- Provider and account id only. No credential columns exist on this table.
SELECT toolkit_slug, connected_account_id, status, connected_at
FROM user_composio_connection
WHERE user_id = $1
ORDER BY connected_at, id;

-- name: ExportListUserChannelBindings :many
-- Channel type and the platform-native user id only; config is omitted.
SELECT channel_type, channel_user_id, workspace_id, bound_at
FROM channel_user_binding
WHERE multica_user_id = $1
ORDER BY bound_at, id;

-- name: ExportListUserWeknoraAccounts :many
-- The WeKnora login id only. password_ciphertext and revealed_at are never selected.
SELECT workspace_id, weknora_user_id, created_at
FROM knowledge_owner_account
WHERE user_id = $1
ORDER BY created_at, workspace_id;
