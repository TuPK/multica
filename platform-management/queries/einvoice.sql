-- name: InsertEinvoiceIssuance :one
-- The revision is max+1 for the order; the unique index on (order_id, revision)
-- rejects a racing insert.
INSERT INTO einvoice_issuance (order_id, revision, workspace_id, state, provider, currency, amount, vat_amount, buyer)
SELECT sqlc.arg('order_id')::uuid,
       COALESCE(MAX(revision), 0) + 1,
       sqlc.arg('workspace_id')::uuid,
       'pending',
       sqlc.arg('provider')::text,
       sqlc.arg('currency')::text,
       sqlc.arg('amount')::bigint,
       sqlc.arg('vat_amount')::bigint,
       sqlc.arg('buyer')::jsonb
FROM einvoice_issuance
WHERE order_id = sqlc.arg('order_id')::uuid
RETURNING *;

-- name: GetEinvoiceIssuance :one
SELECT * FROM einvoice_issuance WHERE order_id = $1 AND revision = $2;

-- name: MarkEinvoiceIssued :execrows
UPDATE einvoice_issuance
SET state = 'issued',
    provider_invoice_id = sqlc.arg('provider_invoice_id')::text,
    invoice_number = sqlc.arg('invoice_number')::text,
    lookup_url = sqlc.arg('lookup_url')::text,
    issued_at = sqlc.arg('issued_at')::timestamptz,
    updated_at = now()
WHERE order_id = sqlc.arg('order_id') AND revision = sqlc.arg('revision') AND state = 'pending';

-- name: CancelEinvoiceIssued :execrows
UPDATE einvoice_issuance
SET state = 'cancelled',
    cancelled_at = now(),
    cancelled_by = sqlc.arg('cancelled_by')::text,
    cancel_reason = sqlc.arg('cancel_reason')::text,
    updated_at = now()
WHERE order_id = sqlc.arg('order_id') AND revision = sqlc.arg('revision') AND state = 'issued';

-- name: ListDueEinvoiceIssuance :many
SELECT * FROM einvoice_issuance
WHERE state = 'pending' AND next_attempt_at <= $1
ORDER BY next_attempt_at, id
LIMIT $2;
