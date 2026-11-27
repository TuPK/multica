-- name: ReachEffectCounts :many
-- Effects in [window_start, window_end) by source, target, decision and whether an agent made
-- them. The Reach Map groups these into surfaces (durable spec 11.2).
SELECT source, target, decision, (agent_id IS NOT NULL)::boolean AS by_agent, COUNT(*)::bigint AS n
FROM effect_event
WHERE workspace_id = sqlc.arg(workspace_id)
    AND created_at >= sqlc.arg(window_start)::timestamptz
    AND created_at < sqlc.arg(window_end)::timestamptz
GROUP BY source, target, decision, by_agent;

-- name: ReachAskCounts :many
-- Attention asks in [window_start, window_end) whose subject is an effect intent, by the
-- intent's source and target (the surface), the ask's reason and squad. Minutes are the
-- actual time when the person answered, else the estimate. The newest ask ids are kept.
SELECT e.source, e.target, a.reason, a.squad_id,
    COUNT(*)::bigint AS n,
    COALESCE(SUM(COALESCE(a.actual_seconds / 60.0, a.estimated_minutes)), 0)::float8 AS minutes,
    ((ARRAY_AGG(a.id ORDER BY a.created_at DESC))[1:5])::uuid[] AS top_ask_ids
FROM attention_ledger a
JOIN effect_intent e ON e.id = a.subject_id AND e.workspace_id = a.workspace_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
    AND a.subject_type = 'effect_intent'
    AND a.created_at >= sqlc.arg(window_start)::timestamptz
    AND a.created_at < sqlc.arg(window_end)::timestamptz
GROUP BY e.source, e.target, a.reason, a.squad_id;

-- name: UpsertReachSnapshotRow :exec
-- One surface's row for one day. Running the same day again rewrites the row. gap_issue_id is
-- left alone: CloseGap sets it, and a nightly run must not forget an issue already opened.
INSERT INTO reach_snapshot (
    workspace_id, snapshot_date, surface, state, effects_30d, agent_effects_30d,
    human_minutes_30d, gap_asks_30d, top_ask_ids, computed_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(snapshot_date), sqlc.arg(surface), sqlc.arg(state),
    sqlc.arg(effects_30d), sqlc.arg(agent_effects_30d), sqlc.arg(human_minutes_30d),
    sqlc.arg(gap_asks_30d), sqlc.arg(top_ask_ids), sqlc.arg(computed_at)
)
ON CONFLICT (workspace_id, snapshot_date, surface) DO UPDATE SET
    state = EXCLUDED.state,
    effects_30d = EXCLUDED.effects_30d,
    agent_effects_30d = EXCLUDED.agent_effects_30d,
    human_minutes_30d = EXCLUDED.human_minutes_30d,
    gap_asks_30d = EXCLUDED.gap_asks_30d,
    top_ask_ids = EXCLUDED.top_ask_ids,
    computed_at = EXCLUDED.computed_at;

-- name: DeleteReachSnapshotsBefore :execrows
-- Keeps 400 days of snapshots in every workspace (durable spec 11.2).
DELETE FROM reach_snapshot WHERE snapshot_date < sqlc.arg(cutoff)::date;

-- name: LatestReachSnapshotDate :one
SELECT MAX(snapshot_date)::date AS snapshot_date FROM reach_snapshot WHERE workspace_id = sqlc.arg(workspace_id);

-- name: ListReachSnapshotDay :many
SELECT * FROM reach_snapshot
WHERE workspace_id = sqlc.arg(workspace_id) AND snapshot_date = sqlc.arg(snapshot_date)
ORDER BY human_minutes_30d DESC, surface;

-- name: GetReachSnapshotRow :one
SELECT * FROM reach_snapshot
WHERE workspace_id = sqlc.arg(workspace_id) AND snapshot_date = sqlc.arg(snapshot_date) AND surface = sqlc.arg(surface);

-- name: LatestReachGapIssueID :one
-- The issue already opened for a surface: the gap_issue_id of its newest row that has one.
SELECT gap_issue_id FROM reach_snapshot
WHERE workspace_id = sqlc.arg(workspace_id) AND surface = sqlc.arg(surface) AND gap_issue_id IS NOT NULL
ORDER BY snapshot_date DESC
LIMIT 1;

-- name: SetReachGapIssue :exec
-- Records the gap issue on the surface's row of the newest snapshot day.
UPDATE reach_snapshot r SET gap_issue_id = sqlc.arg(gap_issue_id)
WHERE r.workspace_id = sqlc.arg(workspace_id)
    AND r.surface = sqlc.arg(surface)
    AND r.snapshot_date = (
        SELECT MAX(l.snapshot_date) FROM reach_snapshot l WHERE l.workspace_id = sqlc.arg(workspace_id)
    );
