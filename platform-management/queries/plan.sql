-- The plan catalogue (spec 5.5). The plan table has no foreign key; a plan that
-- subscriptions name is archived, never deleted, and the delete route checks
-- that in application code.

-- name: ListPlans :many
SELECT * FROM plan
ORDER BY sort, key;

-- name: GetPlan :one
SELECT * FROM plan
WHERE key = $1;

-- name: GetPlanForUpdate :one
SELECT * FROM plan
WHERE key = $1
FOR UPDATE;

-- name: PlanDeletedBefore :one
-- The console writes this row when staff delete a plan; the seed reads it so a
-- deleted plan is not restored.
SELECT EXISTS (
    SELECT 1 FROM audit_event
    WHERE action = 'plan.changed' AND target_type = 'plan' AND target_id = $1
      AND detail->>'deleted' = 'true'
);

-- name: InsertPlanIfAbsent :execrows
-- Seeds one plan. Two platform processes starting together may both see an
-- empty table; the unique index on key makes the second insert a no-op.
INSERT INTO plan (
    key, name, limits, features, currency, seat_price_month, seat_price_year,
    seats_fixed, min_seats, trial_days, is_public, sort, archived_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (key) DO NOTHING;

-- name: UpsertPlan :exec
INSERT INTO plan (
    key, name, limits, features, currency, seat_price_month, seat_price_year,
    seats_fixed, min_seats, trial_days, is_public, sort, archived_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (key) DO UPDATE
SET name = EXCLUDED.name,
    limits = EXCLUDED.limits,
    features = EXCLUDED.features,
    currency = EXCLUDED.currency,
    seat_price_month = EXCLUDED.seat_price_month,
    seat_price_year = EXCLUDED.seat_price_year,
    seats_fixed = EXCLUDED.seats_fixed,
    min_seats = EXCLUDED.min_seats,
    trial_days = EXCLUDED.trial_days,
    is_public = EXCLUDED.is_public,
    sort = EXCLUDED.sort,
    archived_at = EXCLUDED.archived_at;

-- name: DeletePlan :execrows
DELETE FROM plan WHERE key = $1;

-- name: CountSubscriptionsOnPlan :one
-- Every row counts, whatever its status or whether its workspace still exists:
-- a plan that any row names must stay readable.
SELECT count(*)::bigint FROM workspace_subscription
WHERE plan_key = $1;
