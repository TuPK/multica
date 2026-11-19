-- Account deletion (platform management, spec 4.6). Row-by-row cleanup lives
-- here because none of these tables carries a foreign key to "user".

-- name: ListUserWorkspaceCounts :many
-- Every workspace the user belongs to, with the member count the deletion
-- rules need. Ordered so the job and the preview agree.
SELECT m.id AS member_id, w.id AS workspace_id, w.name AS workspace_name, m.role,
       (SELECT count(*) FROM member x WHERE x.workspace_id = w.id)::int AS member_count
FROM member m
JOIN workspace w ON w.id = m.workspace_id
WHERE m.user_id = $1
ORDER BY w.created_at, w.id;

-- name: GetAccountDeletionSchedule :one
-- Status and deletion date; a missing row means an active account.
SELECT status, deletion_scheduled_at
FROM platform_account
WHERE user_id = $1;

-- name: RequestAccountDeletion :one
-- Moves an active account to pending_deletion. No row comes back when the
-- account is not active, so a repeated or concurrent request changes nothing.
INSERT INTO platform_account (user_id, status, reason, changed_by, changed_at, sessions_valid_after, deletion_scheduled_at)
VALUES (@user_id, 'pending_deletion', 'user_requested', @user_id, now(), @sessions_valid_after, @deletion_scheduled_at)
ON CONFLICT (user_id) DO UPDATE SET
    status = 'pending_deletion',
    reason = 'user_requested',
    changed_by = EXCLUDED.changed_by,
    changed_at = now(),
    sessions_valid_after = EXCLUDED.sessions_valid_after,
    deletion_scheduled_at = EXCLUDED.deletion_scheduled_at
WHERE platform_account.status = 'active'
RETURNING deletion_scheduled_at;

-- name: CancelAccountDeletion :one
-- Keeps sessions_valid_after, so the sessions revoked by the request stay revoked.
UPDATE platform_account
SET status = 'active',
    reason = 'deletion_canceled',
    changed_by = user_id,
    changed_at = now(),
    deletion_scheduled_at = NULL
WHERE user_id = $1 AND status = 'pending_deletion'
RETURNING user_id;

-- name: ListDueAccountDeletions :many
-- At most row_limit accounts that are pending and due, oldest first.
SELECT user_id
FROM platform_account
WHERE status = 'pending_deletion' AND deletion_scheduled_at <= @now::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM platform_deletion_request r
    WHERE r.kind = 'account' AND r.target_id = platform_account.user_id
      AND r.state IN ('pending', 'running', 'failed')
  )
ORDER BY deletion_scheduled_at, user_id
LIMIT @row_limit;

-- name: LockDueAccountDeletion :one
-- The job's re-check inside its transaction. No row means the account was
-- canceled, already deleted, or is not due yet.
SELECT user_id, status, deletion_scheduled_at
FROM platform_account
WHERE user_id = @user_id AND status = 'pending_deletion' AND deletion_scheduled_at <= @now::timestamptz
FOR UPDATE;

-- name: MarkAccountDeleted :exec
UPDATE platform_account
SET status = 'deleted', reason = 'account_deleted', changed_at = now()
WHERE user_id = $1 AND status = 'pending_deletion';

-- name: AnonymizeDeletedUser :exec
UPDATE "user"
SET name = 'Deleted user',
    email = 'deleted+' || id::text || '@deleted.invalid',
    avatar_url = NULL,
    profile_description = '',
    onboarding_questionnaire = '{}'::jsonb,
    cloud_waitlist_email = NULL,
    cloud_waitlist_reason = NULL,
    updated_at = now()
WHERE id = $1;

-- name: AnonymizeAccountRequestsForUser :exec
-- Platform account requests that named the user (platform spec 7b.5, rule 11).
-- The personal values are overwritten and the rows stay, so the request ids and
-- the staff who asked are kept. The email gets the address the user row gets.
UPDATE platform_account_request
SET email = 'deleted+' || user_id::text || '@deleted.invalid',
    display_name = 'Deleted user',
    updated_at = now()
WHERE user_id = $1;

-- name: DeleteUserPersonalAccessTokens :many
-- Returns token_hash so the caller can drop the auth cache entries.
DELETE FROM personal_access_token WHERE user_id = $1 RETURNING token_hash;

-- name: DeleteUserDaemonTokens :many
DELETE FROM daemon_token WHERE owner_id = $1 RETURNING token_hash;

-- name: DeleteUserTaskTokens :exec
DELETE FROM task_token WHERE user_id = $1;

-- name: DeleteUserComposioConnections :exec
DELETE FROM user_composio_connection WHERE user_id = $1;

-- name: DeleteUserChannelBindings :exec
DELETE FROM channel_user_binding WHERE multica_user_id = $1;

-- name: DeleteUserLarkBindings :exec
DELETE FROM lark_user_binding WHERE multica_user_id = $1;

-- name: DeleteUserKnowledgeOwnerAccounts :exec
DELETE FROM knowledge_owner_account WHERE user_id = $1;

-- name: DeleteUserConsents :exec
DELETE FROM user_consent WHERE user_id = $1;

-- name: DeleteUserFeedback :exec
DELETE FROM feedback WHERE user_id = $1;

-- name: DeleteUserClientUsage :exec
DELETE FROM client_usage_daily WHERE user_id = $1;

-- name: DeleteUserNotificationPreferences :exec
DELETE FROM notification_preference WHERE user_id = $1;

-- name: DeleteUserPinnedItems :exec
DELETE FROM pinned_item WHERE user_id = $1;

-- name: DeleteUserChatPinnedAgents :exec
DELETE FROM chat_pinned_agent WHERE user_id = $1;

-- name: DeleteUserIssueUnread :exec
DELETE FROM issue_unread WHERE user_id = $1;

-- name: DeleteUserDelegationDays :exec
DELETE FROM member_delegation_day WHERE user_id = $1;

-- name: DeleteUserAutopilotCollaborators :exec
DELETE FROM autopilot_collaborator WHERE user_type = 'member' AND user_id = $1;

-- name: DeleteUserInvitations :exec
-- Invitations addressed to the user, by account or by the email they held.
DELETE FROM workspace_invitation
WHERE invitee_user_id = @user_id OR lower(invitee_email) = lower(@email::text);
