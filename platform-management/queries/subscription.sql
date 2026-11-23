-- Workspace subscriptions (spec 5.5), and the reads the billing routes make on
-- the shared Delegora tables: member (role and head count) and workspace.

-- name: GetWorkspaceSubscription :one
SELECT * FROM workspace_subscription
WHERE workspace_id = $1;

-- name: GetWorkspaceSubscriptionForUpdate :one
-- Locks the row for the rest of the transaction. A missing row locks nothing;
-- the unique index on workspace_id settles two concurrent first writes.
SELECT * FROM workspace_subscription
WHERE workspace_id = $1
FOR UPDATE;

-- name: InsertWorkspaceSubscription :one
-- The first write of a workspace: version 1, the step from "no row" (0). It
-- returns no row when another transaction wrote first.
INSERT INTO workspace_subscription (
    workspace_id, plan_key, status, seats, "interval", current_period_start,
    current_period_end, trial_end, grace_until, cancel_at_period_end,
    limit_overrides, pending_change, unit_price, version
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 1
)
ON CONFLICT (workspace_id) DO NOTHING
RETURNING *;

-- name: UpdateWorkspaceSubscription :one
-- Every change moves version up by exactly one.
UPDATE workspace_subscription
SET plan_key = $2,
    seats = $3,
    "interval" = $4,
    current_period_start = $5,
    current_period_end = $6,
    trial_end = $7,
    grace_until = $8,
    cancel_at_period_end = $9,
    limit_overrides = $10,
    pending_change = $11,
    unit_price = $12,
    version = version + 1,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: CountWorkspacesByPlan :many
-- A workspace with no subscription row is on the free plan.
SELECT COALESCE(s.plan_key, 'free')::text AS plan_key, count(*)::bigint AS workspaces
FROM workspace w
LEFT JOIN workspace_subscription s ON s.workspace_id = w.id
GROUP BY COALESCE(s.plan_key, 'free');

-- name: GetMemberRole :one
SELECT role FROM member
WHERE workspace_id = $1 AND user_id = $2;

-- name: CountWorkspaceMembers :one
SELECT count(*)::bigint FROM member
WHERE workspace_id = $1;
