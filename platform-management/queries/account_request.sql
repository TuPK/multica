-- Account requests (spec 7b.5): an admin's request to create an account for a
-- person. The platform writes the request and cancels it; Delegora's
-- account_provisioning job moves it on.

-- name: InsertAccountRequest :one
-- Inserts a pending request unless a live one (pending or provisioned) holds the
-- email. The conflict target is the partial unique index of migration 968; a
-- lost race returns no row, and the caller reads the winner.
INSERT INTO platform_account_request (
    id, kind, email, display_name, workspace_name, workspace_slug, plan_key,
    seats, interval, trial, comp_reason, billing_profile, created_by
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (email) WHERE state IN ('pending', 'provisioned') DO NOTHING
RETURNING *;

-- name: GetLiveAccountRequestByEmail :one
SELECT * FROM platform_account_request
WHERE email = $1 AND state IN ('pending', 'provisioned');

-- name: GetAccountRequest :one
SELECT * FROM platform_account_request
WHERE id = $1;

-- name: LockAccountRequest :one
SELECT * FROM platform_account_request
WHERE id = $1
FOR UPDATE;

-- name: CancelAccountRequest :one
UPDATE platform_account_request
SET state = 'canceled', updated_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING *;

-- name: ListAccountRequests :many
-- The table is small and written by people, so a page is the newest rows and
-- needs no cursor.
SELECT * FROM platform_account_request
WHERE (sqlc.arg(state)::text = '' OR state = sqlc.arg(state)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetUserIDByEmailIgnoringCase :one
-- Delegora's user table, read to refuse an email that already has an account.
SELECT id FROM "user"
WHERE lower(email) = $1
LIMIT 1;

-- Step 3 of spec 7b.5, rule 7 (the finalize job), and the welcome email of rule 9.
-- Delegora's step 2 owns state, user_id, workspace_id, error_code, attempts and
-- updated_at up to 'provisioned'; the platform moves a provisioned request on.

-- name: ListProvisionedAccountRequestIDs :many
-- A request without a user or a workspace is not ready: step 2 writes both with
-- the state, so none should exist; it is skipped rather than half finished.
SELECT id FROM platform_account_request
WHERE state = 'provisioned' AND user_id IS NOT NULL AND workspace_id IS NOT NULL
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: LockProvisionedAccountRequest :one
-- SKIP LOCKED: a parallel run that holds the row, or a request another run has
-- already finished, returns no row.
SELECT * FROM platform_account_request
WHERE id = $1 AND state = 'provisioned' AND user_id IS NOT NULL AND workspace_id IS NOT NULL
FOR UPDATE SKIP LOCKED;

-- name: CompleteAccountRequest :one
-- Clears the copy of the billing profile, a third party's data, once it is
-- written to the workspace.
UPDATE platform_account_request
SET state = 'completed', billing_profile = NULL, error_code = '',
    completed_at = sqlc.arg(completed_at), updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'provisioned'
RETURNING *;

-- name: RecordAccountRequestFinalizeFailure :one
-- One failed attempt. The request goes to failed with finalize_failed when the
-- error is permanent or this was the last attempt; otherwise it stays provisioned
-- for the next run. The user and the workspace are not touched.
UPDATE platform_account_request
SET attempts = attempts + 1,
    state = CASE WHEN sqlc.arg(permanent)::boolean OR attempts + 1 >= sqlc.arg(max_attempts)::int
                 THEN 'failed' ELSE state END,
    error_code = CASE WHEN sqlc.arg(permanent)::boolean OR attempts + 1 >= sqlc.arg(max_attempts)::int
                      THEN 'finalize_failed' ELSE error_code END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'provisioned'
RETURNING *;

-- name: ListWelcomeDueAccountRequestIDs :many
SELECT id FROM platform_account_request
WHERE state = 'completed' AND welcome_sent_at IS NULL AND welcome_attempts < sqlc.arg(max_attempts)::int
ORDER BY completed_at, id
LIMIT sqlc.arg(row_limit);

-- name: LockWelcomeDueAccountRequest :one
-- The lock is held while the email is sent, so a parallel run skips the row and
-- the welcome goes out once.
SELECT * FROM platform_account_request
WHERE id = sqlc.arg(id) AND state = 'completed' AND welcome_sent_at IS NULL
  AND welcome_attempts < sqlc.arg(max_attempts)::int
FOR UPDATE SKIP LOCKED;

-- name: MarkAccountRequestWelcomeSent :one
UPDATE platform_account_request
SET welcome_sent_at = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkAccountRequestWelcomeFailed :one
UPDATE platform_account_request
SET welcome_attempts = welcome_attempts + 1, updated_at = now()
WHERE id = $1
RETURNING *;
