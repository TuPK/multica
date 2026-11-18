-- E-invoice records of paid orders (platform-management spec 7.3, 7.5).
-- einvoice_issuance belongs to the platform; the server reads it, and its only
-- write is the buyer of a record that needs information or failed. Both queries
-- list their columns so that the generated row does not depend on the table model.

-- name: GetLatestEinvoiceForOrder :one
-- The highest revision of the order's invoice. No row means no invoice exists.
SELECT id, state, invoice_number, lookup_url, buyer, missing_fields
FROM einvoice_issuance
WHERE workspace_id = $1 AND order_id = $2
ORDER BY revision DESC
LIMIT 1;

-- name: SetEinvoiceBuyer :execrows
-- Writes the buyer of a record that is waiting for it and sends the record back
-- to the issue job. It writes no row for any other state, so an issued record is
-- never changed. missing_fields is cleared; the job checks the fields again.
UPDATE einvoice_issuance
SET buyer = $3, missing_fields = '[]'::jsonb, state = 'pending', updated_at = now()
WHERE workspace_id = $1 AND id = $2 AND state IN ('needs_info', 'failed');
