-- Account requests (spec 7b.5): an admin's request to create an account for a
-- person. The platform writes the request and cancels it; Multica's
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
-- Multica's user table, read to refuse an email that already has an account.
SELECT id FROM "user"
WHERE lower(email) = $1
LIMIT 1;
