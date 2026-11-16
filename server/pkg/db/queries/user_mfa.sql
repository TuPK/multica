-- name: GetUserMFA :one
SELECT user_id, secret_enc, confirmed_at, recovery_hashes, last_used_step, created_at
FROM user_mfa
WHERE user_id = $1;

-- name: UpsertPendingUserMFA :one
-- Starts or restarts an enrolment. A confirmed row is left alone, and the
-- caller sees no row: replacing a confirmed secret takes disabling it first.
INSERT INTO user_mfa (user_id, secret_enc)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET secret_enc = EXCLUDED.secret_enc,
    recovery_hashes = '{}',
    last_used_step = 0,
    created_at = now()
WHERE user_mfa.confirmed_at IS NULL
RETURNING user_id, secret_enc, confirmed_at, recovery_hashes, last_used_step, created_at;

-- name: ConfirmUserMFA :execrows
-- Only an unconfirmed row confirms, once. last_used_step takes the step of the
-- confirming code, so that code cannot also sign in.
UPDATE user_mfa
SET confirmed_at = now(),
    recovery_hashes = $2,
    last_used_step = $3
WHERE user_id = $1
  AND confirmed_at IS NULL
  AND secret_enc = $4;

-- name: ClaimUserMFAStep :execrows
-- Records a TOTP step as used. Zero rows means the step, or a later one, was
-- already used.
UPDATE user_mfa
SET last_used_step = $2
WHERE user_id = $1
  AND confirmed_at IS NOT NULL
  AND last_used_step < $2;

-- name: ConsumeUserMFARecoveryHash :execrows
-- Removes one recovery hash. Zero rows means it was not there, or was just used.
UPDATE user_mfa
SET recovery_hashes = array_remove(recovery_hashes, sqlc.arg(recovery_hash)::text)
WHERE user_id = sqlc.arg(user_id)
  AND confirmed_at IS NOT NULL
  AND sqlc.arg(recovery_hash)::text = ANY(recovery_hashes);

-- name: DeleteUserMFA :execrows
-- Disable checks for a confirmed row before calling this. Account deletion
-- calls it too, and must also remove an unconfirmed enrolment's secret.
DELETE FROM user_mfa WHERE user_id = $1;
