-- The plan price-change schedule (spec 7b.4, decision D46). The platform writes
-- plan_price_change; a plan has at most one scheduled row, which the partial
-- unique index of migration 965 holds even for a writer that skipped the plan
-- row lock.

-- name: InsertPlanPriceChange :one
INSERT INTO plan_price_change (plan_key, effective_from, old, new, reason, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetScheduledPlanPriceChange :one
SELECT * FROM plan_price_change
WHERE plan_key = $1 AND state = 'scheduled';

-- name: ListPlanPriceChanges :many
SELECT * FROM plan_price_change
WHERE plan_key = $1
ORDER BY created_at DESC, id;

-- name: ListDuePlanPriceChanges :many
SELECT * FROM plan_price_change
WHERE state = 'scheduled' AND effective_from <= $1
ORDER BY effective_from, id
LIMIT $2;

-- name: ListPlanPriceChangesToNotify :many
SELECT * FROM plan_price_change
WHERE state = 'scheduled' AND notice_sent_at IS NULL
ORDER BY effective_from, id
LIMIT $1;

-- name: MarkPlanPriceChangeApplied :one
-- Applies only to a row that is still scheduled and due: a second or parallel
-- run finds no row.
UPDATE plan_price_change
SET state = 'applied', applied_at = $2
WHERE id = $1 AND state = 'scheduled' AND effective_from <= $2
RETURNING *;

-- name: CancelScheduledPlanPriceChange :one
UPDATE plan_price_change
SET state = 'canceled', canceled_by = $2, canceled_at = $3
WHERE plan_key = $1 AND state = 'scheduled'
RETURNING *;

-- name: MarkPlanPriceChangeNoticeSent :exec
UPDATE plan_price_change
SET notice_sent_at = $2
WHERE id = $1 AND notice_sent_at IS NULL;

-- name: SetPlanPrices :exec
-- Writes the price fields that are given and keeps the others, and moves the
-- plan's version up by one.
UPDATE plan
SET seat_price_month = COALESCE(sqlc.narg(seat_price_month)::bigint, seat_price_month),
    seat_price_year = COALESCE(sqlc.narg(seat_price_year)::bigint, seat_price_year),
    usage_rates = COALESCE(sqlc.narg(usage_rates)::jsonb, usage_rates),
    usage_terms = COALESCE(sqlc.narg(usage_terms)::jsonb, usage_terms),
    version = version + 1
WHERE key = sqlc.arg(key);

-- name: ListPaidWorkspacesOnPlan :many
-- Workspaces whose subscription on the plan is active or past_due, and whether
-- the workspace is pending deletion.
SELECT s.workspace_id,
       COALESCE(w.status = 'pending_deletion', false)::boolean AS pending_deletion
FROM workspace_subscription s
LEFT JOIN platform_workspace w ON w.workspace_id = s.workspace_id
WHERE s.plan_key = $1 AND s.status IN ('active', 'past_due')
ORDER BY s.workspace_id;

-- name: ListWorkspaceOwnerEmails :many
-- Owners only, read from the shared member and user tables.
SELECT u.email::text AS email
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = $1 AND m.role = 'owner' AND u.email <> ''
ORDER BY m.created_at, u.email;
