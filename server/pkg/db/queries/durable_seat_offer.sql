-- The seat offers of a Hub org (durable spec 13.3, E4). Every query filters by workspace_id.

-- name: CreateSeatOffer :one
INSERT INTO seat_offer (id, workspace_id, name, role, squad_id, runtime_id, outcome_criteria, rationale, created_by)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(name), sqlc.arg(role), sqlc.arg(squad_id),
        sqlc.arg(runtime_id), sqlc.arg(outcome_criteria), sqlc.arg(rationale), sqlc.arg(created_by))
RETURNING *;

-- name: GetSeatOfferInWorkspace :one
SELECT * FROM seat_offer
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);
