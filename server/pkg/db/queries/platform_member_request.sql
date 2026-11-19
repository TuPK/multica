-- platform_member_request: the platform writes a staff add-member request; the
-- member_provisioning job owns state, error_code, attempts, user_id, member_id,
-- user_created, updated_at and completed_at.

-- name: ListPendingMemberRequestIDs :many
SELECT id FROM platform_member_request
WHERE state = 'pending' AND kind IN ('add', 'remove', 'change_role')
ORDER BY created_at, id
LIMIT $1;

-- name: LockPendingMemberRequest :one
-- SKIP LOCKED: no row means another run holds the request or it is no longer pending.
SELECT * FROM platform_member_request
WHERE id = $1 AND state = 'pending' AND kind IN ('add', 'remove', 'change_role')
FOR UPDATE SKIP LOCKED;

-- name: MarkMemberRequestCompleted :exec
UPDATE platform_member_request
SET state = 'completed', user_id = $2, member_id = $3, user_created = $4,
    error_code = '', updated_at = now(), completed_at = now()
WHERE id = $1 AND state = 'pending';

-- name: MarkMemberRequestFailed :exec
UPDATE platform_member_request
SET state = 'failed', error_code = $2, updated_at = now()
WHERE id = $1 AND state = 'pending';

-- name: IncrementMemberRequestAttempts :one
UPDATE platform_member_request
SET attempts = attempts + 1, updated_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING attempts;

-- name: DeleteWorkspaceMemberRequests :exec
-- The requests of a workspace go with it: the table holds the email of people
-- who were never members.
DELETE FROM platform_member_request WHERE workspace_id = $1;

-- name: AnonymizeMemberRequestsForUser :exec
-- Platform member requests that named the user (platform spec R6 section 4.3).
-- The email is overwritten with the address the user row gets, and the row stays
-- as the record of who asked.
UPDATE platform_member_request
SET email = 'deleted+' || $1::uuid::text || '@deleted.invalid',
    updated_at = now()
WHERE user_id = $1 OR target_user_id = $1;
