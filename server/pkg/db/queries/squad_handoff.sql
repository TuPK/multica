-- Squad handoff rules and handoffs. Every query filters by workspace_id.

-- name: GetSquadHandoffRule :one
SELECT * FROM squad_handoff_rule
WHERE workspace_id = $1 AND source_squad_id = $2;

-- name: UpsertSquadHandoffRule :one
INSERT INTO squad_handoff_rule (workspace_id, source_squad_id, target_squad_id, mode, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id, source_squad_id) DO UPDATE
SET target_squad_id = EXCLUDED.target_squad_id,
    mode = EXCLUDED.mode,
    updated_by = EXCLUDED.updated_by,
    updated_at = now()
RETURNING *;

-- name: DeleteSquadHandoffRule :exec
DELETE FROM squad_handoff_rule WHERE workspace_id = $1 AND source_squad_id = $2;

-- name: GetSquadHandoffForIssue :one
SELECT * FROM squad_handoff
WHERE workspace_id = $1 AND source_issue_id = $2;

-- name: GetSquadHandoffByTargetIssue :one
SELECT * FROM squad_handoff
WHERE workspace_id = $1 AND target_issue_id = $2;

-- name: InsertSquadHandoff :one
-- Single-flight: a request already handed off, pending, or declined returns no row.
INSERT INTO squad_handoff (
    workspace_id, source_squad_id, target_squad_id, source_issue_id, mode, status,
    requested_by_type, requested_by_id, note
)
VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8)
ON CONFLICT (workspace_id, source_issue_id) DO NOTHING
RETURNING *;

-- name: SettleSquadHandoff :one
UPDATE squad_handoff
SET status = $3, target_issue_id = $4, decided_by = $5, note = $6, decided_at = now()
WHERE workspace_id = $1 AND id = $2
RETURNING *;

-- name: RetargetPendingSquadHandoff :one
-- A pending or declined handoff is decided again against the rule's current target.
UPDATE squad_handoff
SET status = 'pending', target_squad_id = $3, mode = $4, decided_by = NULL, decided_at = NULL
WHERE workspace_id = $1 AND id = $2 AND status <> 'handed_off'
RETURNING *;

-- name: DeletePendingSquadHandoff :exec
-- Withdraws a handoff row an automatic handoff filed and could not run, so a later request starts clean.
DELETE FROM squad_handoff WHERE workspace_id = $1 AND id = $2 AND status = 'pending';

-- name: GetSquadOpenProject :one
-- The project a squad is working on now, with no project excluded.
SELECT * FROM project
WHERE workspace_id = $1 AND squad_id = $2 AND status NOT IN ('completed', 'cancelled')
ORDER BY created_at DESC
LIMIT 1;
