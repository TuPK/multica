-- The seat protocol (spec 5.6, Appendix B): the platform's ledger of seat
-- operations, and the reads of the shared member and subscription tables the
-- capacity decision needs.

-- name: LockSeatWorkspace :exec
-- A transaction-scoped advisory lock in the two-key form, namespace first. It
-- cannot collide with Delegora's WorkspaceLocker, which takes a one-key bigint
-- session lock (Appendix B rule 6).
SELECT pg_advisory_xact_lock($1::int, $2::int);

-- name: GetSeatOperation :one
SELECT * FROM platform_seat_operation
WHERE workspace_id = $1 AND operation_token = $2;

-- name: InsertSeatOperation :one
-- The unique index on (workspace_id, operation_token) settles a second insert.
INSERT INTO platform_seat_operation (
    workspace_id, operation_token, kind, subject_id, state, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (workspace_id, operation_token) DO NOTHING
RETURNING *;

-- name: SetSeatOperationState :one
UPDATE platform_seat_operation
SET state = $3, reason = $4, expires_at = $5, updated_at = now()
WHERE workspace_id = $1 AND operation_token = $2
RETURNING *;

-- name: CountHeldSeatOperations :one
-- Operations that hold a seat: reserved or consuming, and not past expires_at.
SELECT count(*)::bigint FROM platform_seat_operation
WHERE workspace_id = $1
  AND state IN ('reserved', 'consuming')
  AND (expires_at IS NULL OR expires_at > $2);
