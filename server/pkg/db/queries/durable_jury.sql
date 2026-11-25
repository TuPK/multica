-- name: InsertJuryPanel :execrows
-- A panel for one criterion of an outcome. A panel that exists already is kept, so
-- opening one again does nothing (durable spec 8.3).
INSERT INTO jury_panel (id, workspace_id, outcome_id, criterion_id, juror_agent_ids, diversity, state)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(outcome_id), sqlc.arg(criterion_id),
    sqlc.arg(juror_agent_ids)::uuid[], sqlc.arg(diversity), sqlc.arg(state)
)
ON CONFLICT (outcome_id, criterion_id) DO NOTHING;

-- name: GetJuryPanel :one
SELECT * FROM jury_panel
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: GetJuryPanelForCriterion :one
SELECT * FROM jury_panel
WHERE workspace_id = sqlc.arg(workspace_id) AND outcome_id = sqlc.arg(outcome_id)
  AND criterion_id = sqlc.arg(criterion_id);

-- name: ListJuryPanelsForOutcome :many
-- The panels of one outcome. A criterion with a panel is assigned to the jury, so the
-- check job does not give it to a single verifier, and settle reads its panel state.
SELECT * FROM jury_panel
WHERE workspace_id = sqlc.arg(workspace_id) AND outcome_id = sqlc.arg(outcome_id)
ORDER BY created_at, id;

-- name: ListJuryVotesForPanel :many
-- The juror checks on one criterion of an outcome, oldest first.
SELECT * FROM outcome_verification
WHERE workspace_id = sqlc.arg(workspace_id) AND outcome_id = sqlc.arg(outcome_id)
  AND criterion_id = sqlc.arg(criterion_id) AND verifier_type = 'agent'
ORDER BY created_at, id;

-- name: AddJuryJuror :execrows
-- A replacement juror joins an open panel.
UPDATE jury_panel
SET juror_agent_ids = juror_agent_ids || ARRAY[sqlc.arg(agent_id)::uuid]
WHERE id = sqlc.arg(id) AND state = 'open';

-- name: SetJuryPanelState :execrows
-- Moves a panel from one of from_states to state. Zero rows means another writer moved
-- it first, so the caller does nothing more.
UPDATE jury_panel
SET state = sqlc.arg(state), verdict = sqlc.narg(verdict),
    resolved_by_type = sqlc.narg(resolved_by_type), closed_at = sqlc.narg(closed_at)
WHERE id = sqlc.arg(id) AND state = ANY(sqlc.arg(from_states)::text[]);

-- name: ListStaleOpenJuryPanels :many
-- Open panels created before the cutoff, for the 24 hour timeout.
SELECT * FROM jury_panel
WHERE state = 'open' AND created_at < sqlc.arg(cutoff)
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: SetJurorTaskContext :execrows
-- Marks a juror's verification task with its panel. Only a task not yet claimed is
-- marked, as SetAgentTaskDurableContext does for a single verifier.
UPDATE agent_task_queue
SET context = COALESCE(context, '{}'::jsonb)
    || jsonb_build_object(
        'durable_kind', 'verification',
        'outcome_id', sqlc.arg(outcome_id)::text,
        'jury_panel_id', sqlc.arg(jury_panel_id)::text)
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'dispatched', 'deferred');

-- name: CountJuryVotesForTrack :one
-- A juror's votes on resolved panels of one work class since a time: agreeing with the
-- person's verdict, and overruled by it (durable spec 8.3). Unable votes count for neither.
SELECT
    COUNT(*) FILTER (WHERE v.result = p.verdict)::int AS n_pass,
    COUNT(*) FILTER (WHERE v.result <> p.verdict)::int AS n_fail
FROM outcome_verification v
JOIN jury_panel p ON p.outcome_id = v.outcome_id AND p.criterion_id = v.criterion_id
JOIN work_outcome o ON o.id = p.outcome_id
WHERE v.workspace_id = sqlc.arg(workspace_id) AND v.verifier_id = sqlc.arg(juror_agent_id)
  AND v.verifier_type = 'agent' AND v.result IN ('pass', 'partial', 'fail')
  AND p.state = 'resolved' AND p.closed_at >= sqlc.arg(since)::timestamptz
  AND o.work_class = sqlc.arg(work_class);
