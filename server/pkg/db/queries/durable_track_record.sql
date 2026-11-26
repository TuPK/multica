-- Track record and verification modes (durable spec 3.2, 3.3, A6). Rows are keyed by
-- (workspace, agent, work class, window, model); model is '' for the merged row. The
-- mode lives on the 30-day row. Stats are rewritten by recompute; mode and pin columns
-- are written only by the mode statements below.

-- name: CountSettledOutcomesForTrack :one
-- Settled outcomes of one agent's work class, settled at or after window_start. An
-- unverified outcome is counted apart and is not in the score (spec 3.2). Pending
-- outcomes are not settled, so they are never counted.
SELECT
    COUNT(*) FILTER (WHERE verdict = 'pass')::bigint AS n_pass,
    COUNT(*) FILTER (WHERE verdict = 'partial')::bigint AS n_partial,
    COUNT(*) FILTER (WHERE verdict = 'fail')::bigint AS n_fail,
    COUNT(*) FILTER (WHERE verdict = 'unverified')::bigint AS n_unverified,
    COUNT(*) FILTER (WHERE verdict = 'fail' AND severity = 'high')::bigint AS n_high_fail,
    MAX(settled_at) FILTER (WHERE verdict = 'fail' AND severity = 'high')::timestamptz AS last_high_fail_at
FROM work_outcome
WHERE workspace_id = sqlc.arg(workspace_id)
    AND agent_id = sqlc.arg(agent_id)
    AND work_class = sqlc.arg(work_class)
    AND settled_at IS NOT NULL
    AND settled_at >= sqlc.arg(window_start)::timestamptz;

-- name: EnsureTrackRecordRow :exec
-- The 30-day row of a pair, with no stats and the default mode, if it is not there.
INSERT INTO track_record (workspace_id, agent_id, work_class, window_days, model)
VALUES (sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(work_class), 30, '')
ON CONFLICT (workspace_id, agent_id, work_class, window_days, model) DO NOTHING;

-- name: UpsertTrackRecordStats :exec
-- Rewrites the stats of one window. The mode and pin columns are left alone.
INSERT INTO track_record (
    workspace_id, agent_id, work_class, window_days, model,
    n_pass, n_partial, n_fail, n_unverified, n_high_fail, score_lower, last_high_fail_at, computed_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(work_class), sqlc.arg(window_days), '',
    sqlc.arg(n_pass), sqlc.arg(n_partial), sqlc.arg(n_fail), sqlc.arg(n_unverified), sqlc.arg(n_high_fail),
    sqlc.arg(score_lower), sqlc.narg(last_high_fail_at), now()
)
ON CONFLICT (workspace_id, agent_id, work_class, window_days, model) DO UPDATE SET
    n_pass = EXCLUDED.n_pass,
    n_partial = EXCLUDED.n_partial,
    n_fail = EXCLUDED.n_fail,
    n_unverified = EXCLUDED.n_unverified,
    n_high_fail = EXCLUDED.n_high_fail,
    score_lower = EXCLUDED.score_lower,
    last_high_fail_at = EXCLUDED.last_high_fail_at,
    computed_at = EXCLUDED.computed_at;

-- name: GetTrackRecord :one
SELECT * FROM track_record
WHERE workspace_id = sqlc.arg(workspace_id)
    AND agent_id = sqlc.arg(agent_id)
    AND work_class = sqlc.arg(work_class)
    AND window_days = sqlc.arg(window_days)
    AND model = '';

-- name: SetTrackRecordMode :exec
-- Changes the mode of the 30-day row and stamps when it changed (the 7-day rule).
UPDATE track_record
SET verification_mode = sqlc.arg(verification_mode), mode_changed_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
    AND agent_id = sqlc.arg(agent_id)
    AND work_class = sqlc.arg(work_class)
    AND window_days = 30
    AND model = '';

-- name: SetTrackRecordPinned :exec
UPDATE track_record
SET mode_pinned = sqlc.arg(mode_pinned)
WHERE workspace_id = sqlc.arg(workspace_id)
    AND agent_id = sqlc.arg(agent_id)
    AND work_class = sqlc.arg(work_class)
    AND window_days = 30
    AND model = '';

-- name: ListTrackRecords :many
SELECT * FROM track_record
WHERE workspace_id = sqlc.arg(workspace_id)
    AND (sqlc.narg(agent_id)::uuid IS NULL OR agent_id = sqlc.narg(agent_id)::uuid)
    AND (sqlc.narg(work_class)::text IS NULL OR work_class = sqlc.narg(work_class)::text)
    AND (sqlc.narg(window_days)::int IS NULL OR window_days = sqlc.narg(window_days)::int)
    AND model = ''
ORDER BY agent_id, work_class, window_days
LIMIT sqlc.arg(row_limit);

-- name: ListSettledTrackPairsSince :many
-- Agent pairs with an outcome settled at or after since: the nightly job recomputes these.
SELECT DISTINCT workspace_id, agent_id, work_class
FROM work_outcome
WHERE agent_id IS NOT NULL
    AND settled_at IS NOT NULL
    AND settled_at >= sqlc.arg(since)::timestamptz
ORDER BY workspace_id, agent_id, work_class;

-- name: GetAgentRuntimeModel :one
-- The model and runtime provider an agent has now. A missing agent in this workspace is
-- no row. The outcome copies these when it is created (spec 2.3).
SELECT COALESCE(a.model, '')::text AS model, COALESCE(r.provider, '')::text AS provider
FROM agent a
LEFT JOIN agent_runtime r ON r.id = a.runtime_id
WHERE a.id = sqlc.arg(id) AND a.workspace_id = sqlc.arg(workspace_id);

-- name: ListTrackModeChangesBetween :many
-- The pairs whose 30-day verification mode changed in a brief's period, newest first,
-- with the agent's name for the brief (durable spec 3.3).
SELECT tr.agent_id, a.name AS agent_name, tr.work_class, tr.verification_mode, tr.mode_changed_at
FROM track_record tr
JOIN agent a ON a.id = tr.agent_id
WHERE tr.workspace_id = sqlc.arg('workspace_id')::uuid
    AND tr.window_days = 30
    AND tr.mode_changed_at >= sqlc.arg('period_start')::timestamptz
    AND tr.mode_changed_at <= sqlc.arg('period_end')::timestamptz
ORDER BY tr.mode_changed_at DESC, tr.agent_id
LIMIT sqlc.arg('row_limit')::int;
