-- name: ListIssueTransitionsAfterCursor :many
-- Transitions after the outcome cursor, oldest first. Categories come from
-- issue_status (D32); a built-in key with no catalog row is NULL and the Go side
-- maps it (LifecycleCategoryOf). A transition whose issue is gone is skipped.
SELECT
    t.id, t.workspace_id, t.issue_id, t.from_status_key, t.to_status_key, t.created_at,
    i.assignee_type, i.assignee_id,
    fs.category AS from_catalog_category,
    ts.category AS to_catalog_category
FROM issue_status_transition t
JOIN issue i ON i.id = t.issue_id AND i.workspace_id = t.workspace_id
LEFT JOIN issue_status fs ON fs.workspace_id = t.workspace_id AND fs.key = t.from_status_key
LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
WHERE (t.created_at, t.id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY t.created_at, t.id
LIMIT sqlc.arg(row_limit);

-- name: ListIssueLabelNames :many
SELECT l.name
FROM issue_to_label il
JOIN issue_label l ON l.id = il.label_id
WHERE il.issue_id = sqlc.arg(issue_id)
ORDER BY l.name;

-- name: CreateWorkOutcome :one
-- A transition yields at most one outcome (unique done_transition_id). A replay
-- gets the existing row back and writes nothing new.
INSERT INTO work_outcome (
    workspace_id, issue_id, done_transition_id, agent_id, member_id, squad_id,
    work_class, verification_mode, sampled, settle_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(issue_id), sqlc.arg(done_transition_id),
    sqlc.narg(agent_id), sqlc.narg(member_id), sqlc.narg(squad_id),
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

-- name: SettleWorkOutcome :execrows
UPDATE work_outcome
SET verdict = sqlc.arg(verdict), severity = sqlc.arg(severity), fail_cause = sqlc.arg(fail_cause),
    settled_at = sqlc.arg(settled_at), updated_at = now()
WHERE id = sqlc.arg(id) AND settled_at IS NULL;
