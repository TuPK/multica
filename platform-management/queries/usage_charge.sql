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
