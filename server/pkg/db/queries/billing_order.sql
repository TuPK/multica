-- Plans and orders of the platform's billing (platform-management spec 5.8).
-- plan and billing_order belong to the platform; the server reads them, and its
-- only write is the buyer of a pending order.

-- name: ListPublicPlans :many
-- The plans a workspace can choose from, in display order.
SELECT * FROM plan
WHERE is_public AND archived_at IS NULL
ORDER BY sort, key;

-- name: ListWorkspaceBillingOrders :many
SELECT * FROM billing_order
WHERE workspace_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: GetWorkspaceBillingOrder :one
SELECT * FROM billing_order
WHERE workspace_id = $1 AND id = $2;

-- name: SetPendingBillingOrderBuyer :one
-- Writes the buyer of a pending order of the workspace. It returns no row for any
-- other order, so a paid, expired or canceled order, or another workspace's, is
-- never changed.
UPDATE billing_order
SET buyer = $3
WHERE workspace_id = $1 AND id = $2 AND status = 'pending'
RETURNING *;

-- name: GetWorkspaceBillingProfile :one
SELECT * FROM billing_profile
WHERE workspace_id = $1;

-- name: ListUsageChargesForOrder :many
-- The day lines of a usage invoice, in day order and the meters' fixed order.
SELECT day, meter, quantity, unit_price, per, discount_pct, net_amount, note
FROM usage_charge
WHERE order_id = $1
ORDER BY day,
    CASE meter WHEN 'tasks_run' THEN 1 WHEN 'storage_bytes' THEN 2 WHEN 'members' THEN 3 ELSE 4 END,
    meter;
