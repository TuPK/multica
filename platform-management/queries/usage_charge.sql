-- The usage charge (platform management spec 7b.3, rules 8 to 10). The platform
-- writes usage_charge from platform_usage_daily; the usage_rating job reads the
-- plan's rates and terms as they are when it runs.

-- name: ListUsageDaysToRate :many
-- The closed days (before today, UTC) that have a usage snapshot row and no
-- charge row yet, for workspaces on a usage plan. A day is written as a whole
-- in one transaction, so a day with any charge row is done. The workspace's
-- plan, account kind and verification are read as they are now.
SELECT
    u.workspace_id,
    u.day,
    u.members,
    u.tasks_run,
    u.storage_bytes,
    s.plan_key,
    p.usage_rates,
    p.usage_terms,
    COALESCE(bp.account_kind, 'individual')::text AS account_kind,
    (bp.verified_at IS NOT NULL)::boolean AS verified,
    COALESCE(w.status, 'active')::text AS workspace_status
FROM platform_usage_daily u
JOIN workspace_subscription s ON s.workspace_id = u.workspace_id
JOIN plan p ON p.key = s.plan_key AND p.billing_model = 'usage'
LEFT JOIN billing_profile bp ON bp.workspace_id = u.workspace_id
LEFT JOIN platform_workspace w ON w.workspace_id = u.workspace_id
WHERE u.day < sqlc.arg('today')::date
  AND NOT EXISTS (
      SELECT 1 FROM usage_charge c
      WHERE c.workspace_id = u.workspace_id AND c.day = u.day
  )
ORDER BY u.day, u.workspace_id
LIMIT sqlc.arg('row_limit');

-- name: InsertUsageCharge :execrows
-- A (workspace, day, meter) that already has a row keeps it: the first rating
-- stands, so a rerun or a parallel run writes nothing.
INSERT INTO usage_charge (
    workspace_id, day, meter, quantity, plan_key, unit_price, per, discount_pct, net_amount, note
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('day'), sqlc.arg('meter'), sqlc.arg('quantity'),
    sqlc.arg('plan_key'), sqlc.arg('unit_price'), sqlc.arg('per'), sqlc.arg('discount_pct'),
    sqlc.arg('net_amount'), sqlc.arg('note')
)
ON CONFLICT (workspace_id, day, meter) DO NOTHING;

-- The usage invoice (spec 7b.3, rules 11 and 12).

-- name: ListUsageInvoiceWorkspaces :many
-- Workspaces with usage charges not yet on an invoice: a usage plan's rows of
-- days before $1 (the monthly invoice), and any row of a workspace that is no
-- longer on a usage plan (the final invoice).
SELECT s.workspace_id
FROM workspace_subscription s
JOIN plan p ON p.key = s.plan_key
WHERE EXISTS (
    SELECT 1 FROM usage_charge c
    WHERE c.workspace_id = s.workspace_id AND c.order_id IS NULL
      AND (p.billing_model <> 'usage' OR c.day < sqlc.arg('month_start')::date)
)
ORDER BY s.workspace_id;

-- name: LockUsageInvoiceSubscription :one
-- Serialises the invoicing of one workspace until the transaction ends.
SELECT s.plan_key, p.billing_model
FROM workspace_subscription s
JOIN plan p ON p.key = s.plan_key
WHERE s.workspace_id = $1
FOR UPDATE OF s;

-- name: ListUninvoicedUsageCharges :many
-- The rows an invoice would take, locked. A null before takes all of them.
SELECT day, net_amount
FROM usage_charge
WHERE workspace_id = sqlc.arg('workspace_id') AND order_id IS NULL
  AND (sqlc.narg('before')::date IS NULL OR day < sqlc.narg('before')::date)
ORDER BY day
FOR UPDATE;

-- name: AttachUsageCharges :execrows
-- Puts the rows of the given days on an invoice. The condition on order_id
-- keeps a row that is already invoiced where it is.
UPDATE usage_charge
SET order_id = sqlc.arg('order_id')
WHERE workspace_id = sqlc.arg('workspace_id') AND order_id IS NULL
  AND day = ANY(sqlc.arg('days')::date[]);

-- name: GetUsageAccrual :one
-- What the workspace has used and not yet been invoiced for.
SELECT COALESCE(sum(net_amount), 0)::bigint AS accrued
FROM usage_charge
WHERE workspace_id = $1 AND order_id IS NULL;

-- name: GetUsageTerms :one
-- The rates, terms and account facts of a workspace's plan, as they are now.
SELECT
    s.plan_key,
    p.billing_model,
    p.currency,
    p.usage_rates,
    p.usage_terms,
    COALESCE(bp.account_kind, 'individual')::text AS account_kind,
    (bp.verified_at IS NOT NULL)::boolean AS verified
FROM workspace_subscription s
JOIN plan p ON p.key = s.plan_key
LEFT JOIN billing_profile bp ON bp.workspace_id = s.workspace_id
WHERE s.workspace_id = $1;

-- name: ListUsageChargesInRange :many
-- The rows of days from $2 up to, not including, $3, oldest first.
SELECT day, meter, quantity, unit_price, per, discount_pct, net_amount, note, order_id
FROM usage_charge
WHERE workspace_id = $1 AND day >= $2 AND day < $3
ORDER BY day, meter;

-- name: ListMissingUsageDays :many
-- The days from $2 to $3, both included, with no platform_usage_daily row.
SELECT g.day::date AS day
FROM generate_series($2::date, $3::date, interval '1 day') AS g(day)
WHERE NOT EXISTS (
    SELECT 1 FROM platform_usage_daily u
    WHERE u.workspace_id = $1 AND u.day = g.day::date
)
ORDER BY 1;

-- name: ListUnpaidUsageOrders :many
-- The workspace's pending usage invoices, oldest first.
SELECT id, amount, vat_amount, status, created_at, period_start, period_end
FROM billing_order
WHERE workspace_id = $1 AND kind = 'usage' AND status = 'pending'
ORDER BY created_at, id;
