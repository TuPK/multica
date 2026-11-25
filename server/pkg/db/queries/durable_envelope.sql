-- Autonomy envelope (durable spec 3.4, A7). Every query filters by workspace_id; there are
-- no foreign keys. A level change goes through the service, which writes its governance
-- entry in the same transaction.

-- name: GetSquadEnvelope :one
SELECT * FROM squad_autonomy_envelope
WHERE workspace_id = sqlc.arg('workspace_id')::uuid AND squad_id = sqlc.arg('squad_id')::uuid;

-- name: LockSquadEnvelope :one
-- Serializes the envelope's writers with the squad's level changes.
SELECT * FROM squad_autonomy_envelope
WHERE workspace_id = sqlc.arg('workspace_id')::uuid AND squad_id = sqlc.arg('squad_id')::uuid
FOR UPDATE;

-- name: ListSquadEnvelopes :many
-- Every envelope, for the nightly job.
SELECT * FROM squad_autonomy_envelope ORDER BY workspace_id, squad_id;

-- name: UpsertSquadEnvelope :one
INSERT INTO squad_autonomy_envelope (workspace_id, squad_id, floor_level, ceiling_level, set_by, reason)
VALUES (sqlc.arg('workspace_id'), sqlc.arg('squad_id'), sqlc.arg('floor_level'), sqlc.arg('ceiling_level'),
        sqlc.arg('set_by'), sqlc.arg('reason'))
ON CONFLICT (workspace_id, squad_id) DO UPDATE SET
    floor_level = EXCLUDED.floor_level,
    ceiling_level = EXCLUDED.ceiling_level,
    set_by = EXCLUDED.set_by,
    reason = EXCLUDED.reason,
    updated_at = now()
RETURNING *;

-- name: DeleteSquadEnvelope :execrows
DELETE FROM squad_autonomy_envelope
WHERE workspace_id = sqlc.arg('workspace_id')::uuid AND squad_id = sqlc.arg('squad_id')::uuid;

-- name: WidenSquadEnvelope :execrows
-- A level the boss sets outside the envelope widens it to take the level in (spec 3.4).
-- A level inside it, or a squad with no envelope, matches no row.
UPDATE squad_autonomy_envelope
SET floor_level = LEAST(floor_level, sqlc.arg('level')::integer),
    ceiling_level = GREATEST(ceiling_level, sqlc.arg('level')::integer),
    updated_at = now()
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND squad_id = sqlc.arg('squad_id')::uuid
    AND (sqlc.arg('level')::integer < floor_level OR sqlc.arg('level')::integer > ceiling_level);

-- name: StampSquadEnvelopeAutoChange :exec
-- Records when the machine last moved the squad, which the 14-day rise rule reads.
UPDATE squad_autonomy_envelope
SET last_auto_change_at = sqlc.arg('at')::timestamptz, updated_at = now()
WHERE workspace_id = sqlc.arg('workspace_id')::uuid AND squad_id = sqlc.arg('squad_id')::uuid;

-- name: SquadEnvelopeOutcomeCounts :one
-- A squad's settled outcomes in the 90-day window (spec 3.4). An unverified outcome is not
-- counted, as in the track record. High fails are counted in 30 days and in 90 days.
SELECT
    COUNT(*) FILTER (WHERE verdict = 'pass')::bigint AS n_pass,
    COUNT(*) FILTER (WHERE verdict = 'partial')::bigint AS n_partial,
    COUNT(*) FILTER (WHERE verdict = 'fail')::bigint AS n_fail,
    COUNT(*) FILTER (WHERE verdict = 'fail' AND severity = 'high'
        AND settled_at >= sqlc.arg('since_30')::timestamptz)::bigint AS high_fail_30,
    COUNT(*) FILTER (WHERE verdict = 'fail' AND severity = 'high')::bigint AS high_fail_90
FROM work_outcome
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND squad_id = sqlc.arg('squad_id')::uuid
    AND settled_at IS NOT NULL
    AND settled_at >= sqlc.arg('window_start')::timestamptz
    AND verdict IN ('pass', 'partial', 'fail', 'unverified');

-- name: DeleteWorkspaceSquadAutonomyEnvelope :exec
-- Workspace teardown: envelopes have no foreign keys to cascade.
DELETE FROM squad_autonomy_envelope WHERE workspace_id = $1;

-- name: ListAutoAutonomyChangesSince :many
-- Autonomy changes the envelope made since the time, newest first, for the brief's FYI.
-- total_count is the whole count, repeated on each row.
SELECT subject_id, summary, details, created_at, count(*) OVER ()::int AS total_count
FROM governance_event
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND kind = 'autonomy_changed'
    AND actor_type = 'track_record'
    AND created_at >= sqlc.arg('since')::timestamptz
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit')::int;
