-- Delegation by prediction (durable spec 6.5, F5). Every query filters by workspace_id and member_id;
-- there are no foreign keys.

-- name: InsertPrincipalDelegation :one
INSERT INTO principal_delegation (id, workspace_id, member_id, decision_kind, min_confidence)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(member_id), sqlc.arg(decision_kind),
        sqlc.arg(min_confidence)::numeric)
RETURNING *;

-- name: GetActivePrincipalDelegation :one
SELECT * FROM principal_delegation
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND decision_kind = sqlc.arg(decision_kind) AND disabled_at IS NULL;

-- name: DisablePrincipalDelegation :execrows
UPDATE principal_delegation SET disabled_at = now(), disabled_reason = sqlc.arg(disabled_reason)
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND decision_kind = sqlc.arg(decision_kind) AND disabled_at IS NULL;

-- name: DeletePrincipalDelegations :execrows
DELETE FROM principal_delegation
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);

-- name: GetDecisionPredictionByID :one
SELECT * FROM decision_prediction
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: MarkPredictionAppliedByDelegation :execrows
-- A delegated decision is the prediction, decided now: it resolves with that outcome, and its 72-hour
-- revert window starts from resolved_at.
UPDATE decision_prediction
SET applied_by_delegation = true, actual = predicted, matched = true, resolved_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND resolved_at IS NULL;

-- name: ListRecentDelegatedPredictionIDs :many
-- The last twenty decisions a delegation made for one kind (spec 6.5).
SELECT id FROM decision_prediction
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND decision_kind = sqlc.arg(decision_kind) AND applied_by_delegation = true
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- name: CountDelegationReverts :one
-- How many of the given delegated decisions the person reverted, as the governance log has them.
SELECT COUNT(*)::int FROM governance_event
WHERE workspace_id = sqlc.arg(workspace_id) AND kind = 'principal_delegation_reverted'
  AND subject_type = 'decision_prediction' AND subject_id = ANY(sqlc.arg(subject_ids)::uuid[]);
