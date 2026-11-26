-- Decision predictions (durable spec 6.4, F5). Every query filters by workspace_id and member_id;
-- there are no foreign keys.

-- name: InsertPendingPrediction :execrows
-- A question waiting for its prediction. A question already admitted for the person keeps its row.
INSERT INTO decision_prediction (id, workspace_id, member_id, subject_type, subject_id, decision_kind)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(member_id), sqlc.arg(subject_type), sqlc.arg(subject_id),
        sqlc.arg(decision_kind))
ON CONFLICT (subject_type, subject_id, member_id) DO NOTHING;

-- name: ListProfiledDeciders :many
-- The members who decide for the workspace (owners and admins) and have an active profile.
SELECT p.member_id FROM principal_profile p
JOIN member m ON m.workspace_id = p.workspace_id AND m.user_id = p.member_id
WHERE p.workspace_id = sqlc.arg(workspace_id) AND p.status = 'active' AND m.role IN ('owner', 'admin')
ORDER BY p.member_id;

-- name: RecordDecisionPrediction :one
-- The Secretary's prediction. A resolved question keeps its outcome: no row comes back.
INSERT INTO decision_prediction (id, workspace_id, member_id, subject_type, subject_id, decision_kind,
                                 predicted, confidence, why, trait_ids, profile_version, predictor_task_id)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(member_id), sqlc.arg(subject_type), sqlc.arg(subject_id),
        sqlc.arg(decision_kind), sqlc.arg(predicted), sqlc.arg(confidence)::numeric, sqlc.arg(why),
        sqlc.arg(trait_ids)::uuid[], sqlc.arg(profile_version), sqlc.narg(predictor_task_id)::uuid)
ON CONFLICT (subject_type, subject_id, member_id) DO UPDATE SET
    decision_kind = EXCLUDED.decision_kind,
    predicted = EXCLUDED.predicted,
    confidence = EXCLUDED.confidence,
    why = EXCLUDED.why,
    trait_ids = EXCLUDED.trait_ids,
    profile_version = EXCLUDED.profile_version,
    predictor_task_id = EXCLUDED.predictor_task_id
WHERE decision_prediction.resolved_at IS NULL
RETURNING *;

-- name: GetDecisionPrediction :one
SELECT * FROM decision_prediction
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND subject_type = sqlc.arg(subject_type) AND subject_id = sqlc.arg(subject_id);

-- name: ResolveDecisionPredictions :execrows
-- The person decided the question: every prediction of it that is still open is resolved, and
-- matched says whether the prediction was the decision.
UPDATE decision_prediction
SET actual = sqlc.arg(actual), matched = (predicted = sqlc.arg(actual)), resolved_at = now()
WHERE workspace_id = sqlc.arg(workspace_id) AND subject_type = sqlc.arg(subject_type)
  AND subject_id = sqlc.arg(subject_id) AND resolved_at IS NULL AND predicted <> '';

-- name: ListRecentResolvedPredictions :many
-- The last resolved predictions of one kind, newest first, for the accuracy window (spec 6.4).
SELECT matched FROM decision_prediction
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND decision_kind = sqlc.arg(decision_kind) AND resolved_at IS NOT NULL AND matched IS NOT NULL
ORDER BY resolved_at DESC
LIMIT 50;

-- name: ForgetPredictionReasons :execrows
-- Deleting a profile keeps the match counts and drops what the prediction said (spec 6.6).
UPDATE decision_prediction SET why = '', trait_ids = '{}'
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);
