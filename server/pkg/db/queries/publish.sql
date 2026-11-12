-- =====================
-- Publishing channels (Content Studio)
-- =====================

-- name: ListPublishChannels :many
SELECT * FROM publish_channel
WHERE workspace_id = $1
ORDER BY created_at, id;

-- name: GetPublishChannelInWorkspace :one
SELECT * FROM publish_channel
WHERE id = $1 AND workspace_id = $2;

-- name: InsertPublishChannel :one
INSERT INTO publish_channel (workspace_id, platform, market, account_name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdatePublishChannelStatus :one
UPDATE publish_channel
SET status = $3, status_detail = $4, last_checked_at = $5, updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeletePublishChannel :execrows
DELETE FROM publish_channel
WHERE id = $1 AND workspace_id = $2;

-- =====================
-- Publish settings (per-workspace Ayrshare keys, sealed)
-- =====================

-- name: GetPublishSettings :one
SELECT * FROM publish_settings
WHERE workspace_id = $1;

-- name: UpsertPublishSettings :exec
-- Each key is written only when its set_ flag is true, so a PUT that omits a key
-- leaves the stored key alone. A set flag with a NULL sealed value clears the key.
-- The conflict update reads the stored row, so two saves of different keys do not
-- overwrite each other.
INSERT INTO publish_settings (workspace_id, ayrshare_key_sealed, x_key_sealed, x_secret_sealed)
VALUES (
    sqlc.arg('workspace_id')::uuid,
    CASE WHEN sqlc.arg('set_ayrshare')::boolean THEN sqlc.narg('ayrshare_key_sealed')::bytea END,
    CASE WHEN sqlc.arg('set_x_key')::boolean THEN sqlc.narg('x_key_sealed')::bytea END,
    CASE WHEN sqlc.arg('set_x_secret')::boolean THEN sqlc.narg('x_secret_sealed')::bytea END
)
ON CONFLICT (workspace_id) DO UPDATE SET
    ayrshare_key_sealed = CASE WHEN sqlc.arg('set_ayrshare')::boolean THEN sqlc.narg('ayrshare_key_sealed')::bytea ELSE publish_settings.ayrshare_key_sealed END,
    x_key_sealed        = CASE WHEN sqlc.arg('set_x_key')::boolean THEN sqlc.narg('x_key_sealed')::bytea ELSE publish_settings.x_key_sealed END,
    x_secret_sealed     = CASE WHEN sqlc.arg('set_x_secret')::boolean THEN sqlc.narg('x_secret_sealed')::bytea ELSE publish_settings.x_secret_sealed END,
    updated_at          = now();

-- name: ListPublishConfiguredWorkspaces :many
-- Workspaces with an Ayrshare key. The publisher sends only for these.
SELECT workspace_id FROM publish_settings
WHERE ayrshare_key_sealed IS NOT NULL;

-- name: DeleteWorkspacePublishSettings :exec
-- Workspace teardown only: the stored keys go with their workspace.
DELETE FROM publish_settings WHERE workspace_id = sqlc.arg('workspace_id')::uuid;

-- =====================
-- Publish profile (one Ayrshare User Profile per workspace)
-- =====================

-- name: GetPublishProfile :one
SELECT * FROM publish_profile
WHERE workspace_id = $1;

-- name: InsertPublishProfileIfAbsent :execrows
-- Returns 0 when another request already stored the profile, so the caller can
-- keep the stored key and not overwrite it.
INSERT INTO publish_profile (workspace_id, profile_key_sealed, ref_id)
VALUES ($1, $2, $3)
ON CONFLICT (workspace_id) DO NOTHING;

-- =====================
-- Publish attempts
-- =====================

-- name: InsertPublishAttempt :execrows
-- The idempotency key is (workspace_id, item_id, channel_id, version). A repeat
-- approval of the same version inserts nothing.
INSERT INTO publish_attempt (workspace_id, proposal_id, item_id, channel_id, version, approved_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (workspace_id, item_id, channel_id, version) DO NOTHING;

-- name: ClaimQueuedPublishAttemptInWorkspaces :one
-- Takes the oldest queued attempt of the given workspaces and marks it sending in
-- one statement, so the claim commits before the HTTP call. SKIP LOCKED lets several
-- server instances share the queue without claiming the same row.
UPDATE publish_attempt
SET status = 'sending', updated_at = now()
WHERE id = (
    SELECT id FROM publish_attempt
    WHERE status = 'queued' AND workspace_id = ANY(sqlc.arg('workspace_ids')::uuid[])
    ORDER BY created_at, id
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: RequeueStalePublishAttempts :execrows
-- An attempt left sending by a crash goes back to queued. It is resent with the
-- same idempotency key; Ayrshare then answers a duplicate, which the publisher
-- records as posted.
UPDATE publish_attempt
SET status = 'queued', updated_at = now()
WHERE status = 'sending' AND updated_at < sqlc.arg('stale_before')::timestamptz;

-- name: RequeuePublishAttempt :exec
-- Puts a sending attempt back to queued when its send was interrupted by shutdown.
UPDATE publish_attempt
SET status = 'queued', updated_at = now()
WHERE id = $1 AND status = 'sending';

-- name: MarkPublishAttemptPosted :exec
UPDATE publish_attempt
SET status = 'posted', external_post_id = $2, external_url = $3, error = '', updated_at = now()
WHERE id = $1 AND status = 'sending';

-- name: MarkPublishAttemptFailed :exec
UPDATE publish_attempt
SET status = 'failed', error = $2, updated_at = now()
WHERE id = $1 AND status = 'sending';

-- name: DeleteWorkspacePublishAttempts :exec
-- Workspace teardown only: the send log goes with its workspace.
DELETE FROM publish_attempt WHERE workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: DeleteWorkspacePublishChannels :exec
-- Workspace teardown only: channels go with their workspace.
DELETE FROM publish_channel WHERE workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: DeleteWorkspacePublishProfile :exec
-- Workspace teardown only: the Ayrshare profile key row goes with its workspace.
DELETE FROM publish_profile WHERE workspace_id = sqlc.arg('workspace_id')::uuid;
