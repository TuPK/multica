-- name: GetDurableOutcomeCursor :one
SELECT name, created_at, transition_id, last_seq, pending_seq, pending_xmax, updated_at
FROM durable_outcome_cursor
WHERE name = sqlc.arg(name);

-- name: UpsertDurableOutcomeCursor :exec
-- Moves the cursor. The job calls this in the transaction that wrote the outcomes
-- of the rows it read (durable spec 2.4, V2.2).
INSERT INTO durable_outcome_cursor (name, created_at, transition_id, last_seq, pending_seq, pending_xmax, updated_at)
VALUES (
    sqlc.arg(name),
    sqlc.arg(created_at),
    sqlc.arg(transition_id),
    sqlc.arg(last_seq),
    sqlc.arg(pending_seq),
    sqlc.arg(pending_xmax),
    now()
)
ON CONFLICT (name) DO UPDATE SET
    created_at = EXCLUDED.created_at,
    transition_id = EXCLUDED.transition_id,
    last_seq = EXCLUDED.last_seq,
    pending_seq = EXCLUDED.pending_seq,
    pending_xmax = EXCLUDED.pending_xmax,
    updated_at = now();

-- name: GetCurrentSnapshotBounds :one
-- Returns the current transaction snapshot's oldest active XID (xmin) and next
-- unassigned XID (xmax). Used to advance last_seq only once every transaction
-- that could hold a lower sequence number has committed or rolled back (V2.2).
SELECT
    pg_snapshot_xmin(pg_current_snapshot())::text::bigint AS xmin,
    pg_snapshot_xmax(pg_current_snapshot())::text::bigint AS xmax;
