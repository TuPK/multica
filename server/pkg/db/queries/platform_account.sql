-- name: AccountState :one
SELECT status, sessions_valid_after
FROM platform_account
WHERE user_id = $1;

-- name: SetAccountState :one
INSERT INTO platform_account (user_id, status, reason, changed_by, changed_at, sessions_valid_after, deletion_scheduled_at)
VALUES ($1, $2, $3, $4, now(), $5, $6)
ON CONFLICT (user_id) DO UPDATE SET
    status = EXCLUDED.status,
    reason = EXCLUDED.reason,
    changed_by = EXCLUDED.changed_by,
    changed_at = now(),
    sessions_valid_after = EXCLUDED.sessions_valid_after,
    deletion_scheduled_at = EXCLUDED.deletion_scheduled_at
RETURNING *;

-- name: RevokeSessionsForUser :exec
-- Touches only sessions_valid_after; a new row starts active.
INSERT INTO platform_account (user_id, status, changed_at, sessions_valid_after)
VALUES ($1, 'active', now(), $2)
ON CONFLICT (user_id) DO UPDATE SET
    sessions_valid_after = EXCLUDED.sessions_valid_after;
