-- Billing profiles (spec 7b.1): the account kind and billing identity of a
-- workspace. A workspace with no row is an individual account.

-- name: GetBillingProfile :one
SELECT * FROM billing_profile
WHERE workspace_id = $1;

-- name: LockBillingProfileWrites :exec
-- Serialises the writers of one workspace's profile until the transaction ends.
-- Migration 955 (unique index on workspace_id) is not applied yet, so nothing
-- else stops two first writes from inserting two rows.
SELECT pg_advisory_xact_lock(hashtextextended('billing_profile:' || $1::text, 0));

-- name: InsertBillingProfile :one
INSERT INTO billing_profile (
    workspace_id, account_kind, legal_name, tax_code, address, invoice_email,
    verified_at, verified_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: UpdateBillingProfile :one
UPDATE billing_profile
SET account_kind = $2,
    legal_name = $3,
    tax_code = $4,
    address = $5,
    invoice_email = $6,
    verified_at = $7,
    verified_by = $8,
    updated_by = $9,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;
