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
-- empty table; the unique index on key makes the second insert a no-op. An
-- empty audience or billing model takes the column default; version starts at 0.
-- Absent usage_rates and usage_terms take the column default, an empty object.
INSERT INTO plan (
    key, name, limits, features, currency, seat_price_month, seat_price_year,
    seats_fixed, min_seats, trial_days, is_public, sort, archived_at,
    audience, billing_model, usage_rates, usage_terms
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    COALESCE(NULLIF(sqlc.arg(audience)::text, ''), 'any'), COALESCE(NULLIF(sqlc.arg(billing_model)::text, ''), 'seat'),
    COALESCE(sqlc.narg(usage_rates)::jsonb, '{}'), COALESCE(sqlc.narg(usage_terms)::jsonb, '{}')
)
ON CONFLICT (key) DO NOTHING;

-- name: UpsertPlan :exec
-- Creates the plan, or replaces its fields and moves version up by one. An empty
-- audience or billing model is "not given": a new row takes the default and an
-- existing row keeps its value, so a writer that does not carry the two fields
-- does not reset them. usage_rates and usage_terms are not written here.
INSERT INTO plan (
    key, name, limits, features, currency, seat_price_month, seat_price_year,
    seats_fixed, min_seats, trial_days, is_public, sort, archived_at,
    audience, billing_model
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    COALESCE(NULLIF(sqlc.arg(audience)::text, ''), 'any'), COALESCE(NULLIF(sqlc.arg(billing_model)::text, ''), 'seat')
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
    archived_at = EXCLUDED.archived_at,
    audience = COALESCE(NULLIF(sqlc.arg(audience)::text, ''), plan.audience),
    billing_model = COALESCE(NULLIF(sqlc.arg(billing_model)::text, ''), plan.billing_model),
    version = plan.version + 1;

-- name: DeletePlan :execrows
DELETE FROM plan WHERE key = $1;

-- name: PlanHasSubscriptionsOrOrders :one
-- True when any subscription row or billing order names the plan, whatever its
-- status: the plan's currency is then fixed (spec 7b.4 rule 6).
SELECT (EXISTS (SELECT 1 FROM workspace_subscription ws WHERE ws.plan_key = $1)
    OR EXISTS (SELECT 1 FROM billing_order bo WHERE bo.plan_key = $1))::boolean;

-- name: CountSubscriptionsOnPlan :one
-- Every row counts, whatever its status or whether its workspace still exists:
-- a plan that any row names must stay readable.
SELECT count(*)::bigint FROM workspace_subscription
WHERE plan_key = $1;
