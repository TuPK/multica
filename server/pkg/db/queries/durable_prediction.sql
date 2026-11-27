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
-- The person decided the question: every open row for this subject is resolved. Rows that
-- carried a prediction record whether it matched; pending rows with no prediction keep
-- matched = NULL so they do not skew accuracy, while resolved_at blocks late writes.
UPDATE decision_prediction
SET actual = sqlc.arg(actual),
    matched = CASE WHEN predicted <> '' THEN (predicted = sqlc.arg(actual)) ELSE NULL END,
    resolved_at = now()
WHERE workspace_id = sqlc.arg(workspace_id) AND subject_type = sqlc.arg(subject_type)
  AND subject_id = sqlc.arg(subject_id) AND resolved_at IS NULL;

-- name: InsertResolvedPredictionTombstones :execrows
-- Persists the decision outcome for every active profiled member who has no resolved row
-- yet for this subject, so a late Record after Resolve is refused (V1.8). Race-safe with
-- concurrent Record via ON CONFLICT (subject_type, subject_id, member_id).
INSERT INTO decision_prediction (
    id, workspace_id, member_id, subject_type, subject_id, decision_kind,
    predicted, actual, matched, resolved_at
)
SELECT gen_random_uuid(), p.workspace_id, p.member_id,
       sqlc.arg(subject_type), sqlc.arg(subject_id), 'general',
       '', sqlc.arg(actual), NULL, now()
FROM principal_profile p
WHERE p.workspace_id = sqlc.arg(workspace_id) AND p.status = 'active'
ON CONFLICT (subject_type, subject_id, member_id) DO UPDATE SET
    actual = EXCLUDED.actual,
    matched = CASE WHEN decision_prediction.predicted <> '' THEN (decision_prediction.predicted = EXCLUDED.actual) ELSE NULL END,
    resolved_at = EXCLUDED.resolved_at
WHERE decision_prediction.resolved_at IS NULL;

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

-- name: ListUnlearnedResolvedPredictionsSince :many
-- The unconsumed decisions resolved since a time, newest first, for one profile update batch (spec 6.3).
SELECT id, subject_type, subject_id, decision_kind, predicted, actual, matched, resolved_at
FROM decision_prediction
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND resolved_at IS NOT NULL AND resolved_at >= sqlc.arg(since)
  AND learning_task_id IS NULL
ORDER BY resolved_at DESC, id DESC
LIMIT 20;

-- name: MarkPredictionsLearned :execrows
-- Stamps each resolved decision with the routine task that picked it up. The NULL guard keeps
-- concurrent runs from enqueuing a second task for the same decisions.
UPDATE decision_prediction
SET learning_task_id = sqlc.arg(learning_task_id)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND member_id = sqlc.arg(member_id)
  AND id = ANY(sqlc.arg(ids)::uuid[])
  AND learning_task_id IS NULL;

-- name: ClearPredictionsLearned :execrows
-- Releases the learning marker if task enqueue fails outside a transaction.
UPDATE decision_prediction
SET learning_task_id = NULL
WHERE workspace_id = sqlc.arg(workspace_id)
  AND member_id = sqlc.arg(member_id)
  AND learning_task_id = sqlc.arg(learning_task_id);

-- name: EnqueuePrincipalLearningTask :one
-- Enqueues a routine task for the workspace Secretary to update a member's principal profile
-- (durable spec 6.3, V1.9). Follows row lock order: locks the agent row first, then
-- lock_task_owner_rows locks workspace and runtime.
WITH enqueue_agent AS MATERIALIZED (
    SELECT a.id, a.runtime_id, a.owner_id
    FROM agent a
    WHERE a.id = sqlc.arg(agent_id)
      AND a.workspace_id = sqlc.arg(workspace_id)
      AND a.archived_at IS NULL
      AND a.runtime_id IS NOT NULL
      AND lock_task_owner_rows(a.id, NULL, a.runtime_id)
    FOR KEY SHARE OF a
)
INSERT INTO agent_task_queue (
    id, agent_id, runtime_id, issue_id, status, priority, trigger_summary, context,
    accountable_user_id, originator_source
)
SELECT
    sqlc.arg(id),
    enqueue_agent.id,
    enqueue_agent.runtime_id,
    NULL,
    'queued',
    0,
    sqlc.arg(trigger_summary),
    sqlc.arg(context)::jsonb,
    enqueue_agent.owner_id,
    CASE WHEN enqueue_agent.owner_id IS NOT NULL THEN 'owner_fallback' ELSE 'unattributed' END
FROM enqueue_agent
RETURNING *;

-- name: ListActivePrincipalProfiles :many
-- Every active profile in every workspace, for the profile update job.
SELECT workspace_id, member_id FROM principal_profile WHERE status = 'active';

-- name: LatestAssistantMessageAt :one
-- When the Secretary last wrote into a chat session.
SELECT created_at FROM chat_message
WHERE chat_session_id = sqlc.arg(chat_session_id) AND role = 'assistant'
ORDER BY created_at DESC
LIMIT 1;

-- name: ListPredictionKinds :many
-- The decision kinds the person has predictions of, for the delegation panel's accuracy list.
SELECT DISTINCT decision_kind FROM decision_prediction
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
ORDER BY decision_kind;

