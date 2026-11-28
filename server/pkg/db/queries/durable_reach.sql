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
-- Attention asks in [window_start, window_end) mapped to their surface across all ask
-- subject types (effect_intent, app_action_request, outcome_check, proposal, inquiry),
-- grouped by source, target, reason and squad (durable spec 11.2). Minutes are the
-- actual time when the person answered, else the estimate. The newest ask ids are kept.
SELECT
    mapped.source::text AS source,
    mapped.target::text AS target,
    mapped.reason,
    mapped.squad_id,
    COUNT(*)::bigint AS n,
    COALESCE(SUM(COALESCE(mapped.actual_seconds / 60.0, mapped.estimated_minutes)), 0)::float8 AS minutes,
    ((ARRAY_AGG(mapped.id ORDER BY mapped.created_at DESC))[1:5])::uuid[] AS top_ask_ids
FROM (
    SELECT
        a.id, a.reason, a.squad_id, a.actual_seconds, a.estimated_minutes, a.created_at,
        CASE a.subject_type
            WHEN 'effect_intent' THEN e.source
            WHEN 'app_action_request' THEN 'connected_app'
            WHEN 'outcome_check' THEN 'outcome_check'
            WHEN 'proposal' THEN 'proposal'
            WHEN 'inquiry' THEN 'inquiry'
        END AS source,
        CASE a.subject_type
            WHEN 'effect_intent' THEN e.target
            WHEN 'app_action_request' THEN ar.toolkit_slug
            WHEN 'outcome_check' THEN o.work_class
            WHEN 'proposal' THEN 'squad:' || p.squad_id::text
            WHEN 'inquiry' THEN 'squad:' || bi.squad_id::text
        END AS target
    FROM attention_ledger a
    LEFT JOIN effect_intent e
        ON a.subject_type = 'effect_intent' AND e.id = a.subject_id AND e.workspace_id = a.workspace_id
    LEFT JOIN app_action_request ar
        ON a.subject_type = 'app_action_request' AND ar.id = a.subject_id AND ar.workspace_id = a.workspace_id
    LEFT JOIN work_outcome o
        ON a.subject_type = 'outcome_check' AND o.id = a.subject_id AND o.workspace_id = a.workspace_id
    LEFT JOIN proposal p
        ON a.subject_type = 'proposal' AND p.id = a.subject_id AND p.workspace_id = a.workspace_id
    LEFT JOIN brief_inquiry bi
        ON a.subject_type = 'inquiry' AND bi.id = a.subject_id AND bi.workspace_id = a.workspace_id
    WHERE a.workspace_id = sqlc.arg(workspace_id)
        AND a.created_at >= sqlc.arg(window_start)::timestamptz
        AND a.created_at < sqlc.arg(window_end)::timestamptz
) mapped
WHERE mapped.source IS NOT NULL AND mapped.target IS NOT NULL AND mapped.target <> ''
GROUP BY mapped.source, mapped.target, mapped.reason, mapped.squad_id;

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

-- name: LockReachSnapshotRowsForSurface :many
-- Serializes CloseGap for one surface in a workspace so concurrent calls open at most one issue.
SELECT * FROM reach_snapshot
WHERE workspace_id = sqlc.arg(workspace_id) AND surface = sqlc.arg(surface)
ORDER BY snapshot_date DESC
FOR UPDATE;

-- name: LowestReachGapAsksSince :one
-- Lowest gap_asks_30d recorded for a surface in reach_snapshot since a timestamp (spec 11.3).
SELECT MIN(gap_asks_30d)::numeric AS best FROM reach_snapshot
WHERE workspace_id = sqlc.arg(workspace_id)
    AND surface = sqlc.arg(surface)
    AND computed_at >= sqlc.arg(since)::timestamptz;

-- name: HighestReachGapAsksSince :one
-- Highest gap_asks_30d recorded for a surface in reach_snapshot since a timestamp.
SELECT MAX(gap_asks_30d)::numeric AS best FROM reach_snapshot
WHERE workspace_id = sqlc.arg(workspace_id)
    AND surface = sqlc.arg(surface)
    AND computed_at >= sqlc.arg(since)::timestamptz;
