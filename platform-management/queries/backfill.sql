-- The cutover backfill's batches (gate G-R2).

-- name: ListWorkspacesWithoutSubscription :many
-- The next batch of workspaces with no subscription row, in id order. A row the
-- previous batch inserted drops out of the filter, so the next batch starts at
-- the first workspace still unsubscribed.
SELECT w.id, w.name
FROM workspace w
WHERE NOT EXISTS (
    SELECT 1 FROM workspace_subscription s WHERE s.workspace_id = w.id
)
ORDER BY w.id
LIMIT $1;
