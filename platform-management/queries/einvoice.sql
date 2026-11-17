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

-- name: ListPaidOrdersNeedingEinvoice :many
SELECT o.id, o.workspace_id, o.currency, o.amount, o.vat_amount, o.buyer
FROM billing_order o
WHERE o.status = 'paid'
  AND o.amount > 0
  AND o.provider <> 'admin_grant'
  AND o.paid_at >= sqlc.arg('start_at')::timestamptz
  AND NOT EXISTS (SELECT 1 FROM einvoice_issuance e WHERE e.order_id = o.id)
ORDER BY o.paid_at, o.id
LIMIT sqlc.arg('max_rows')::int;

-- name: ClaimDueEinvoiceIssuance :many
-- Leases due pending rows so a concurrent run skips them. The attempt is
-- counted at claim time, before the provider is called.
UPDATE einvoice_issuance
SET locked_until = sqlc.arg('lease_until')::timestamptz,
    attempts = attempts + 1,
    updated_at = now()
WHERE (order_id, revision) IN (
    SELECT d.order_id, d.revision FROM einvoice_issuance d
    WHERE d.state = 'pending'
      AND d.next_attempt_at <= sqlc.arg('now')::timestamptz
      AND (d.locked_until IS NULL OR d.locked_until <= sqlc.arg('now')::timestamptz)
    ORDER BY d.next_attempt_at, d.id
    LIMIT sqlc.arg('max_rows')::int
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: SetEinvoiceAttempt :execrows
UPDATE einvoice_issuance
SET state = sqlc.arg('state')::text,
    next_attempt_at = sqlc.arg('next_attempt_at')::timestamptz,
    last_error_code = sqlc.arg('last_error_code')::text,
    missing_fields = sqlc.arg('missing_fields')::jsonb,
    locked_until = NULL,
    updated_at = now()
WHERE order_id = sqlc.arg('order_id') AND revision = sqlc.arg('revision') AND state = 'pending';
