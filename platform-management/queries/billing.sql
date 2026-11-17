-- Billing orders (spec 5.8). billing_order has no primary key or foreign key;
-- the unique indexes on id, (workspace_id, idempotency_key) and transfer_code
-- are built by migrations 678 to 680.

-- name: CreateBillingOrder :one
-- Inserts a pending order. An order with the same non-empty idempotency key in
-- the same workspace is not written and the query returns no row; the caller
-- reads the existing order. A transfer code collision is an error, for the
-- caller to retry with a new code.
INSERT INTO billing_order (
    id, workspace_id, kind, plan_key, "interval", seats, amount, vat_amount,
    currency, status, provider, transfer_code, idempotency_key, buyer,
    period_start, period_end, created_by, expires_at, created_at
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7,
    $8, 'pending', $9, $10, $11, $12,
    $13, $14, $15, $16, $17
)
ON CONFLICT (workspace_id, idempotency_key) WHERE idempotency_key <> '' DO NOTHING
RETURNING *;

-- name: GetBillingOrder :one
SELECT * FROM billing_order
WHERE id = $1;

-- name: GetPendingBillingOrderForWorkspace :one
-- The workspace's pending order, if it has one. A change is refused while one
-- is pending (spec 7b.1, rule 9). A usage invoice is a debt with its own rule
-- (spec 7b.3, rule 13), so it does not count.
SELECT * FROM billing_order
WHERE workspace_id = $1 AND status = 'pending' AND kind <> 'usage'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetUnpaidUsageOrderForWorkspace :one
-- The workspace's oldest pending usage invoice: its debt (spec 7b.3, rule 13).
SELECT * FROM billing_order
WHERE workspace_id = $1 AND kind = 'usage' AND status = 'pending'
ORDER BY created_at ASC, id ASC
LIMIT 1;

-- name: CountUsageChargesOfOrder :one
SELECT count(*)::bigint FROM usage_charge
WHERE order_id = $1;

-- name: GetBillingOrderByIdempotencyKey :one
SELECT * FROM billing_order
WHERE workspace_id = $1 AND idempotency_key = $2 AND idempotency_key <> '';

-- name: GetBillingOrderForUpdate :one
-- Locks the order row for the rest of the transaction.
SELECT * FROM billing_order
WHERE id = $1
FOR UPDATE;

-- name: MarkBillingOrderPaid :one
-- Moves a pending order to paid. It returns no row for an order that is not
-- pending; ApplyPayment calls it with the row locked.
UPDATE billing_order
SET status = 'paid',
    paid_at = $2,
    confirmed_by = $3
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ListPendingBillingOrders :many
-- The oldest pending orders first, for the expiry job.
SELECT * FROM billing_order
WHERE status = 'pending'
ORDER BY created_at ASC, id ASC
LIMIT $1;

-- name: ListExpiredPendingBillingOrders :many
-- Pending orders whose expires_at is at or before $1, the longest-expired first,
-- for the order expiry job.
SELECT * FROM billing_order
WHERE status = 'pending' AND expires_at <= $1
ORDER BY expires_at ASC, id ASC
LIMIT $2;

-- name: ExpireBillingOrder :one
-- Moves a pending order whose expires_at is at or before $2 to expired. It
-- returns no row when the order is not pending or has not expired, so a second
-- run, or a payment that got there first, changes nothing.
UPDATE billing_order
SET status = 'expired'
WHERE id = $1 AND status = 'pending' AND expires_at <= $2
RETURNING *;

-- name: ApplyPaymentToWorkspaceSubscription :one
-- The subscription change of a confirmed order. Unlike
-- UpdateWorkspaceSubscription it also writes status, because a payment makes
-- the subscription active. It also writes pending_change (a paid renewal clears
-- it) and unit_price. Moves version up by exactly one.
UPDATE workspace_subscription
SET plan_key = $2,
    seats = $3,
    "interval" = $4,
    current_period_start = $5,
    current_period_end = $6,
    status = $7,
    grace_until = $8,
    pending_change = $9,
    unit_price = $10,
    version = version + 1,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: GetPendingAddSeatsOrder :one
-- The workspace's newest add_seats order that can still be paid. The billing
-- page reads it as the active seat purchase.
SELECT * FROM billing_order
WHERE workspace_id = $1
  AND kind = 'add_seats'
  AND status = 'pending'
  AND expires_at > $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetWorkspaceSlug :one
-- The address of an order page names the workspace by its slug.
SELECT slug FROM workspace
WHERE id = $1;

-- name: CancelBillingOrder :one
-- Moves a pending order to canceled. It returns no row for an order that is not
-- pending; the console calls it with the row locked.
UPDATE billing_order
SET status = 'canceled'
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: GetBillingOrderWithWorkspace :one
SELECT sqlc.embed(o), w.name AS workspace_name
FROM billing_order o
LEFT JOIN workspace w ON w.id = o.workspace_id
WHERE o.id = $1;

-- name: ListBillingOrdersByStatusOldestFirst :many
-- The confirmation queue: the order that has waited longest comes first.
SELECT sqlc.embed(o), w.name AS workspace_name
FROM billing_order o
LEFT JOIN workspace w ON w.id = o.workspace_id
WHERE o.status = sqlc.arg(status)
ORDER BY o.created_at ASC, o.id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListBillingOrdersByStatusNewestFirst :many
SELECT sqlc.embed(o), w.name AS workspace_name
FROM billing_order o
LEFT JOIN workspace w ON w.id = o.workspace_id
WHERE o.status = sqlc.arg(status)
ORDER BY o.created_at DESC, o.id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListBillingOrdersByWorkspace :many
SELECT sqlc.embed(o), w.name AS workspace_name
FROM billing_order o
LEFT JOIN workspace w ON w.id = o.workspace_id
WHERE o.workspace_id = sqlc.arg(workspace_id)
ORDER BY o.created_at DESC, o.id DESC
LIMIT sqlc.arg(row_limit);

-- name: CountPendingBillingOrders :one
SELECT count(*)::bigint FROM billing_order
WHERE status = 'pending';
