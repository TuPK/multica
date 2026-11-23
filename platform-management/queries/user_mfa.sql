-- The second factor Delegora stores for a user (table user_mfa, enrolled in the
-- Delegora web app). The platform reads a confirmed row to check the staff
-- sign-in and deletes a row when staff reset a user's second factor.

-- name: GetConfirmedUserMFA :one
SELECT secret_enc, last_used_step FROM user_mfa
WHERE user_id = $1 AND confirmed_at IS NOT NULL;

-- name: GetAnyConfirmedUserMFA :one
-- One confirmed row, for the start check that the key opens Delegora's values.
SELECT secret_enc FROM user_mfa
WHERE confirmed_at IS NOT NULL
LIMIT 1;

-- name: UseUserMFAStep :execrows
-- Takes a time step once: it changes nothing when the step is not after the one
-- already used, which is how a code is refused a second time.
UPDATE user_mfa
SET last_used_step = $2
WHERE user_id = $1 AND confirmed_at IS NOT NULL AND last_used_step < $2;

-- name: DeleteUserMFA :execrows
DELETE FROM user_mfa
WHERE user_id = $1;
