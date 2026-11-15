-- The subscription lifecycle job (spec 5.9): the subscriptions each edge of the
-- state diagram makes due, the conditional move, and the notice rows.

-- name: ListSubscriptionsTrialEnded :many
-- trialing subscriptions whose trial_end is at or before $1.
SELECT * FROM workspace_subscription
WHERE status = 'trialing' AND trial_end IS NOT NULL AND trial_end <= $1
ORDER BY trial_end, workspace_id
LIMIT $2;

-- name: ListSubscriptionsCanceledLapsed :many
-- active subscriptions that end with the period and whose period is over.
SELECT * FROM workspace_subscription
WHERE status = 'active' AND cancel_at_period_end
  AND current_period_end IS NOT NULL AND current_period_end <= $1
ORDER BY current_period_end, workspace_id
LIMIT $2;

-- name: ListSubscriptionsPeriodLapsed :many
-- active subscriptions that were not renewed by the end of the period.
SELECT * FROM workspace_subscription
WHERE status = 'active' AND NOT cancel_at_period_end
  AND current_period_end IS NOT NULL AND current_period_end <= $1
ORDER BY current_period_end, workspace_id
LIMIT $2;

-- name: ListSubscriptionsGraceEnded :many
SELECT * FROM workspace_subscription
WHERE status = 'past_due' AND grace_until IS NOT NULL AND grace_until <= $1
ORDER BY grace_until, workspace_id
LIMIT $2;

-- name: ListSubscriptionsReadOnlyEnded :many
-- The read-only period is counted from grace_until, which the read_only status
-- keeps. $3 is its length in days.
SELECT * FROM workspace_subscription
WHERE status = 'read_only' AND grace_until IS NOT NULL
  AND grace_until + make_interval(days => $3::int) <= $1
ORDER BY grace_until, workspace_id
LIMIT $2;

-- name: ListSubscriptionsForReminder :many
-- active subscriptions that end after $1 and by $2 and do not stop at the end.
SELECT * FROM workspace_subscription
WHERE status = 'active' AND NOT cancel_at_period_end
  AND current_period_end IS NOT NULL
  AND current_period_end > $1 AND current_period_end <= $2
ORDER BY current_period_end, workspace_id
LIMIT $3;

-- name: MoveWorkspaceSubscription :one
-- One lifecycle step. It applies only to the row the job saw: the same status
-- and the same version. No row comes back when another writer moved it first.
UPDATE workspace_subscription
SET plan_key = $4,
    status = $5,
    seats = $6,
    current_period_start = $7,
    current_period_end = $8,
    grace_until = $9,
    cancel_at_period_end = $10,
    version = version + 1,
    updated_at = now()
WHERE workspace_id = $1 AND status = $2 AND version = $3
RETURNING *;

-- name: GetWorkspaceNameAndSlug :one
SELECT name, slug FROM workspace
WHERE id = $1;

-- name: ListWorkspaceNoticeEmails :many
-- Owners and admins, read from the shared member and user tables.
SELECT u.email::text AS email
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = $1 AND m.role IN ('owner', 'admin') AND u.email <> ''
ORDER BY m.created_at, u.email;

-- name: InsertBillingNotice :execrows
-- Returns 1 when the row is new and 0 when the key was already recorded.
INSERT INTO billing_notice (workspace_id, notice_key, kind)
VALUES ($1, $2, $3)
ON CONFLICT (workspace_id, notice_key) DO NOTHING;

-- name: DeleteBillingNotice :exec
DELETE FROM billing_notice
WHERE workspace_id = $1 AND notice_key = $2;
