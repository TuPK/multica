-- name: GetDurableOutcomeCursor :one
SELECT name, created_at, transition_id, updated_at
FROM durable_outcome_cursor
WHERE name = sqlc.arg(name);

-- name: UpsertDurableOutcomeCursor :exec
-- Moves the cursor. The job calls this in the transaction that wrote the outcomes
-- of the rows it read (durable spec 2.4).
INSERT INTO durable_outcome_cursor (name, created_at, transition_id, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(created_at), sqlc.arg(transition_id), now())
ON CONFLICT (name) DO UPDATE SET
    created_at = EXCLUDED.created_at,
    transition_id = EXCLUDED.transition_id,
    updated_at = now();
