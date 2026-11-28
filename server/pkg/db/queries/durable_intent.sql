-- Rehearsed actions (durable spec 5.2, F4). A state move is a single guarded UPDATE:
-- the WHERE names the state it leaves, so a second decision on the same intent
-- changes no row (the caller answers 409).

-- name: InsertEffectIntent :one
INSERT INTO effect_intent (workspace_id, task_id, agent_id, issue_id, source, target,
    effect_class, reversibility, call, call_sha256, preview, rationale, predicted, expires_at, batch_key)
VALUES (
    sqlc.arg('workspace_id')::uuid,
    sqlc.narg('task_id')::uuid,
    sqlc.narg('agent_id')::uuid,
    sqlc.narg('issue_id')::uuid,
    sqlc.arg('source')::text,
    sqlc.arg('target')::text,
    sqlc.arg('effect_class')::text,
    sqlc.arg('reversibility')::text,
    sqlc.arg('call')::jsonb,
    sqlc.arg('call_sha256')::text,
    sqlc.arg('preview')::jsonb,
    sqlc.arg('rationale')::text,
    sqlc.narg('predicted')::jsonb,
    sqlc.arg('expires_at')::timestamptz,
    sqlc.arg('batch_key')::text
)
RETURNING *;

-- name: GetEffectIntent :one
SELECT * FROM effect_intent
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: GetEffectIntentForUpdate :one
-- Locks the intent for the approval transaction (durable spec 5.3).
SELECT * FROM effect_intent
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid
FOR UPDATE;

-- name: ListEffectIntents :many
SELECT * FROM effect_intent
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('row_limit')::integer;

-- name: ApproveEffectIntent :execrows
-- pending to approved, with the call the platform will run (edited or not) and its hash.
UPDATE effect_intent SET
    state = 'approved',
    call = sqlc.arg('call')::jsonb,
    call_sha256 = sqlc.arg('call_sha256')::text,
    decided_by = sqlc.arg('decided_by')::uuid,
    decided_at = now(),
    decision_note = sqlc.arg('decision_note')::text
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid AND state = 'pending';

-- name: SetEffectIntentOutcome :execrows
-- approved to executed or failed, once the platform has run the call.
UPDATE effect_intent SET state = sqlc.arg('state')::text
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid AND state = 'approved';

-- name: RejectEffectIntent :execrows
UPDATE effect_intent SET
    state = 'rejected',
    decided_by = sqlc.arg('decided_by')::uuid,
    decided_at = now(),
    decision_note = sqlc.arg('decision_note')::text
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid AND state = 'pending';

-- name: ExpirePendingEffectIntents :many
-- Moves a bounded batch of pending intents past their expiry to expired, skipping rows
-- locked by concurrent decisions. The RETURNING rows are the ones this batch expired,
-- so each agent is woken once.
UPDATE effect_intent SET state = 'expired', decided_at = now()
WHERE id IN (
    SELECT id FROM effect_intent
    WHERE state = 'pending' AND expires_at <= sqlc.arg('now')::timestamptz
    ORDER BY expires_at ASC, id ASC
    LIMIT sqlc.arg('row_limit')::integer
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: LinkEffectEventToIntent :exec
UPDATE effect_event SET intent_id = sqlc.arg('intent_id')::uuid
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND status = 'awaiting' AND intent_id IS NULL;

-- name: FinishEffectEventForIntent :execrows
-- The rehearsed action's event follows its intent: awaiting to executed, blocked, or failed.
-- When executed, undo, undo_until and result are recorded in the same transition.
UPDATE effect_event SET
    status = sqlc.arg('status')::text,
    undo = COALESCE(sqlc.narg('undo')::jsonb, undo),
    undo_until = COALESCE(sqlc.narg('undo_until')::timestamptz, undo_until),
    result = COALESCE(sqlc.narg('result')::jsonb, result)
WHERE intent_id = sqlc.arg('intent_id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND status = 'awaiting'
  AND sqlc.arg('status')::text IN ('executed', 'blocked', 'failed');

-- name: CancelEffectEventForIntent :execrows
-- A rejected or expired intent's event goes to blocked: the action did not run.
UPDATE effect_event SET status = 'blocked'
WHERE intent_id = sqlc.arg('intent_id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid AND status = 'awaiting';

-- name: GetEffectEventByIntent :one
-- The event a rehearsal wrote, which names the member the call runs for.
SELECT * FROM effect_event
WHERE intent_id = sqlc.arg('intent_id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid
ORDER BY created_at ASC
LIMIT 1;

-- name: GetLatestEffectIntentForTarget :one
-- The newest intent of one target on an issue: a flow task holds one intent per round (durable spec 5.4).
SELECT * FROM effect_intent
WHERE workspace_id = sqlc.arg('workspace_id')::uuid AND issue_id = sqlc.arg('issue_id')::uuid AND target = sqlc.arg('target')
ORDER BY created_at DESC
LIMIT 1;
