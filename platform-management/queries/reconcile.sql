-- Reconciliation of bank transfer notices (spec 7.2).

-- name: GetBillingOrderByTransferCode :one
SELECT * FROM billing_order
WHERE transfer_code = $1;

-- name: ListReceivedPaymentNoticeIDsBefore :many
-- Notices still in received that arrived at or before $1, oldest first, for the
-- redrive job.
SELECT id FROM payment_notice
WHERE state = 'received' AND received_at <= $1
ORDER BY received_at ASC, id ASC
LIMIT $2;

-- name: DecidePaymentNotice :execrows
-- Moves a notice from from_state to to_state and sets its order. It writes
-- nothing when the notice is no longer in from_state.
UPDATE payment_notice
SET state = sqlc.arg('to_state')::text,
    review_reason = CASE WHEN sqlc.arg('to_state')::text = 'needs_review' THEN sqlc.arg('reason')::text ELSE review_reason END,
    order_id = sqlc.arg('order_id')::text,
    decided_at = CASE WHEN sqlc.arg('to_state')::text = 'needs_review' THEN decided_at ELSE now() END,
    decided_by = CASE WHEN sqlc.arg('to_state')::text = 'needs_review' THEN decided_by ELSE sqlc.arg('decided_by')::text END,
    decision_reason = CASE WHEN sqlc.arg('to_state')::text = 'needs_review' THEN decision_reason ELSE sqlc.arg('reason')::text END
WHERE id = sqlc.arg('id') AND state = sqlc.arg('from_state');
