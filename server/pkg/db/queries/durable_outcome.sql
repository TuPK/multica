-- name: ListIssueTransitionsAfterCursor :many
-- Unapplied transitions after the safe sequence cursor, oldest seq first.
-- Categories come from issue_status (D32); a built-in key with no catalog row is
-- NULL and the Go side maps it (LifecycleCategoryOf). A transition whose issue is
-- gone is skipped.
SELECT
    t.id, t.seq, t.workspace_id, t.issue_id, t.from_status_key, t.to_status_key, t.created_at,
    t.assignee_type, t.assignee_id, t.label_names, w.settings AS workspace_settings,
    COALESCE(p.outcomes_disabled, false)::boolean AS outcomes_disabled,
    fs.category AS from_catalog_category,
    ts.category AS to_catalog_category
FROM issue_status_transition t
JOIN issue i ON i.id = t.issue_id AND i.workspace_id = t.workspace_id
JOIN workspace w ON w.id = t.workspace_id
LEFT JOIN project p ON p.id = i.project_id AND p.workspace_id = t.workspace_id
LEFT JOIN durable_outcome_applied a ON a.transition_id = t.id
LEFT JOIN issue_status fs ON fs.workspace_id = t.workspace_id AND fs.key = t.from_status_key
LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
WHERE t.seq > sqlc.arg(after_seq)::bigint
  AND ((NOT sqlc.arg(retry_failed)::boolean AND a.transition_id IS NULL)
       OR (sqlc.arg(retry_failed)::boolean AND a.status = 'failed'))
ORDER BY t.seq
LIMIT sqlc.arg(row_limit);

-- name: ListIssueLabelNames :many
SELECT l.name
FROM issue_to_label il
JOIN issue_label l ON l.id = il.label_id
WHERE il.issue_id = sqlc.arg(issue_id)
ORDER BY l.name;

-- name: CreateWorkOutcome :one
-- A transition yields at most one outcome (unique done_transition_id). A replay
-- gets the existing row back and writes nothing new. The caller passes the id, so the
-- verification sample (spec 3.3) is chosen from it before the row exists.
INSERT INTO work_outcome (
    id, workspace_id, issue_id, done_transition_id, agent_id, member_id, squad_id,
    runtime_provider, model, work_class, verification_mode, sampled, settle_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(issue_id), sqlc.arg(done_transition_id),
    sqlc.narg(agent_id), sqlc.narg(member_id), sqlc.narg(squad_id),
    sqlc.arg(runtime_provider), sqlc.arg(model),
    sqlc.arg(work_class), sqlc.arg(verification_mode), sqlc.arg(sampled), sqlc.arg(settle_at)
)
ON CONFLICT (done_transition_id) DO UPDATE SET done_transition_id = EXCLUDED.done_transition_id
RETURNING *;

-- name: SupersedeCurrentWorkOutcomes :exec
UPDATE work_outcome
SET superseded_by = sqlc.arg(superseded_by), updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
    AND issue_id = sqlc.arg(issue_id)
    AND superseded_by IS NULL
    AND id <> sqlc.arg(superseded_by);

-- name: ReopenCurrentWorkOutcome :execrows
-- Reopen inside the window only: a settled outcome keeps its verdict (spec 2.4).
UPDATE work_outcome
SET verdict = 'fail', fail_cause = 'reopened', updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
    AND issue_id = sqlc.arg(issue_id)
    AND superseded_by IS NULL
    AND settled_at IS NULL;

-- name: RevertCurrentWorkOutcome :execrows
-- done to cancelled: the current outcome is fail/reverted and settled now (D31).
UPDATE work_outcome
SET verdict = 'fail', fail_cause = 'reverted', settled_at = now(), updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
    AND issue_id = sqlc.arg(issue_id)
    AND superseded_by IS NULL;

-- name: ListDueWorkOutcomes :many
SELECT * FROM work_outcome
WHERE settle_at <= sqlc.arg(due_at)::timestamptz AND settled_at IS NULL
ORDER BY settle_at, id
LIMIT sqlc.arg(row_limit);

-- name: LockWorkOutcomeForSettle :one
SELECT * FROM work_outcome
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: SettleWorkOutcome :execrows
UPDATE work_outcome
SET verdict = sqlc.arg(verdict), severity = sqlc.arg(severity), fail_cause = sqlc.arg(fail_cause),
    settled_at = sqlc.arg(settled_at), updated_at = now()
WHERE id = sqlc.arg(id) AND settled_at IS NULL;

-- name: ClaimOutcomeTransition :execrows
-- Returns 1 when this transaction applies the transition, 0 when an earlier run
-- already did. The caller skips the transition on 0 (durable spec 2.4).
INSERT INTO durable_outcome_applied (transition_id)
VALUES (sqlc.arg(transition_id))
ON CONFLICT (transition_id) DO UPDATE SET status = 'applied'
WHERE durable_outcome_applied.status = 'failed';

-- name: CountWorkOutcomesSettledBetween :one
-- The outcomes settled in a brief's period, by verdict. reopened counts the fails that
-- a reopen made, a subset of fail (durable spec 2.6).
SELECT
    COUNT(*)::bigint AS settled,
    COUNT(*) FILTER (WHERE verdict = 'pass')::bigint AS n_pass,
    COUNT(*) FILTER (WHERE verdict = 'partial')::bigint AS n_partial,
    COUNT(*) FILTER (WHERE verdict = 'fail')::bigint AS n_fail,
    COUNT(*) FILTER (WHERE verdict = 'fail' AND fail_cause = 'reopened')::bigint AS n_reopened,
    COUNT(*) FILTER (WHERE verdict = 'unverified')::bigint AS n_unverified
FROM work_outcome
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND settled_at IS NOT NULL
    AND settled_at >= sqlc.arg('period_start')::timestamptz
    AND settled_at <= sqlc.arg('period_end')::timestamptz;
