-- name: InsertSeatDossierEntry :one
-- A new entry on a seat (durable spec 9.2). The writer has checked the cap and the agent.
INSERT INTO seat_dossier_entry (id, workspace_id, agent_id, kind, text, work_class, evidence, origin)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(kind), sqlc.arg(text),
    sqlc.arg(work_class), sqlc.arg(evidence), sqlc.arg(origin)
)
RETURNING *;

-- name: GetSeatDossierEntry :one
SELECT * FROM seat_dossier_entry
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListSeatDossierEntries :many
-- The entries of a seat, newest first. An empty status lists every status.
SELECT * FROM seat_dossier_entry
WHERE workspace_id = @workspace_id AND agent_id = @agent_id
  AND (@status::text = '' OR status = @status::text)
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: CountActiveSeatDossierEntries :one
SELECT COUNT(*)::int FROM seat_dossier_entry
WHERE workspace_id = @workspace_id AND agent_id = @agent_id AND status = 'active';

-- name: RetireSeatDossierEntry :one
-- Retires an active entry. A retired entry stays in the table, and the briefing skips it.
UPDATE seat_dossier_entry
SET status = 'retired', retired_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND status = 'active'
RETURNING *;

-- name: ListSeatDossierCleanupCandidates :many
-- Seats with at least two active entries, which the monthly cleanup asks to tidy (spec 9.2).
-- Archived agents are left out.
SELECT a.workspace_id, a.id AS agent_id, a.name, COUNT(d.id)::int AS active_entries
FROM seat_dossier_entry d
JOIN agent a ON a.id = d.agent_id AND a.workspace_id = d.workspace_id
WHERE d.status = 'active' AND a.archived_at IS NULL
GROUP BY a.workspace_id, a.id, a.name
HAVING COUNT(d.id) >= 2
ORDER BY a.workspace_id, a.id;
