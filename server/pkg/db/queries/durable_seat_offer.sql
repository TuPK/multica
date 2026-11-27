-- The seat offers of a Hub org (durable spec 13.3, E4). Every query filters by workspace_id.

-- name: CreateSeatOffer :one
INSERT INTO seat_offer (
    id, workspace_id, name, role, work_class, squad_id, runtime_id, outcome_criteria,
    proposed_budget_micros, proposed_budget_period, proposed_envelope_floor, proposed_envelope_ceiling,
    rationale, created_by
)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(name), sqlc.arg(role), sqlc.arg(work_class),
    sqlc.arg(squad_id), sqlc.arg(runtime_id), sqlc.arg(outcome_criteria),
    sqlc.arg(proposed_budget_micros), sqlc.arg(proposed_budget_period),
    sqlc.arg(proposed_envelope_floor), sqlc.arg(proposed_envelope_ceiling),
    sqlc.arg(rationale), sqlc.arg(created_by)
)
RETURNING *;

-- name: GetSeatOfferInWorkspace :one
SELECT * FROM seat_offer
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListSeatOffersByWorkspace :many
SELECT * FROM seat_offer
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: CloseSeatOfferInWorkspace :one
UPDATE seat_offer
SET status = 'closed'
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND status = 'open'
RETURNING *;
