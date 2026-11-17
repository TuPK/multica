-- name: InsertPaymentNotice :one
-- Returns no row when the provider event was already stored.
INSERT INTO payment_notice (provider, provider_event_id, state, transfer_code, amount, currency, body_sha256, received_at)
VALUES ($1, $2, 'received', $3, $4, $5, $6, COALESCE(sqlc.narg('received_at')::timestamptz, now()))
ON CONFLICT (provider, provider_event_id) DO NOTHING
RETURNING *;

-- name: GetPaymentNoticeByEvent :one
SELECT * FROM payment_notice WHERE provider = $1 AND provider_event_id = $2;

-- name: GetPaymentNotice :one
SELECT * FROM payment_notice WHERE id = $1;

-- name: TransitionPaymentNotice :execrows
UPDATE payment_notice
SET state = sqlc.arg('to_state'),
    review_reason = CASE WHEN sqlc.arg('to_state') = 'needs_review' THEN sqlc.arg('review_reason')::text ELSE review_reason END,
    decided_at = CASE WHEN sqlc.arg('to_state') = 'needs_review' THEN decided_at ELSE now() END,
    decided_by = CASE WHEN sqlc.arg('to_state') = 'needs_review' THEN decided_by ELSE sqlc.arg('decided_by')::text END,
    decision_reason = CASE WHEN sqlc.arg('to_state') = 'needs_review' THEN decision_reason ELSE sqlc.arg('decision_reason')::text END
WHERE id = sqlc.arg('id') AND state = sqlc.arg('from_state');

-- name: ListPaymentNoticesNeedingReview :many
SELECT * FROM payment_notice
WHERE state = 'needs_review'
ORDER BY received_at ASC, id ASC
LIMIT $1;
