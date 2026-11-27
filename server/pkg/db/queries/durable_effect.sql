-- The effect log (durable spec 4.4). Append-only: the updates are the status moves,
-- the undo fields and the intent link (see durable_intent.sql); there is no DELETE
-- query (TestEffectEventAppendOnly).

-- name: InsertEffectEvent :one
INSERT INTO effect_event (workspace_id, task_id, agent_id, member_id, source, target,
    effect_class, reversibility, decision, status, summary, intent_id, app_tool_call_id)
VALUES (
    sqlc.arg('workspace_id')::uuid,
    sqlc.narg('task_id')::uuid,
    sqlc.narg('agent_id')::uuid,
    sqlc.narg('member_id')::uuid,
    sqlc.arg('source')::text,
    sqlc.arg('target')::text,
    sqlc.arg('effect_class')::text,
    sqlc.arg('reversibility')::text,
    sqlc.arg('decision')::text,
    sqlc.arg('status')::text,
    sqlc.arg('summary')::text,
    sqlc.narg('intent_id')::uuid,
    sqlc.narg('app_tool_call_id')::uuid
)
RETURNING id;

-- name: FinishEffectEvent :execrows
-- Moves an awaiting event to its outcome. undo, undo_until and result are set
-- when the forward call succeeded.
UPDATE effect_event SET
    status = sqlc.arg('status')::text,
    undo = COALESCE(sqlc.narg('undo')::jsonb, undo),
    undo_until = COALESCE(sqlc.narg('undo_until')::timestamptz, undo_until),
    result = COALESCE(sqlc.narg('result')::jsonb, result)
WHERE id = sqlc.arg('id')::uuid
    AND workspace_id = sqlc.arg('workspace_id')::uuid
    AND status = 'awaiting';

-- name: CountAgentSendsSince :one
-- Executed sends by one agent since a time: the L3 daily cap (durable spec 4.3).
SELECT count(*)::integer FROM effect_event
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND agent_id = sqlc.arg('agent_id')::uuid
    AND effect_class = 'send'
    AND status = 'executed'
    AND created_at >= sqlc.arg('since')::timestamptz;

-- name: GetEffectEvent :one
SELECT * FROM effect_event
WHERE id = sqlc.arg('id')::uuid AND workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: ListEffectEvents :many
-- Newest first, filtered by any of the optional filters. before is the created_at of
-- the last row of the previous page.
SELECT * FROM effect_event
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
    AND (sqlc.narg('effect_class')::text IS NULL OR effect_class = sqlc.narg('effect_class')::text)
    AND (sqlc.narg('source')::text IS NULL OR source = sqlc.narg('source')::text)
    AND (sqlc.narg('decision')::text IS NULL OR decision = sqlc.narg('decision')::text)
    AND (sqlc.narg('agent_id')::uuid IS NULL OR agent_id = sqlc.narg('agent_id')::uuid)
    AND (sqlc.narg('task_id')::uuid IS NULL OR task_id = sqlc.narg('task_id')::uuid)
    AND (sqlc.narg('before')::timestamptz IS NULL OR created_at < sqlc.narg('before')::timestamptz)
ORDER BY created_at DESC
LIMIT sqlc.arg('row_limit')::integer;

-- name: ClaimEffectEventUndo :execrows
-- Claims an executed effect before calling the inverse tool so concurrent undo
-- requests cannot invoke the external provider twice.
UPDATE effect_event SET status = 'undoing'
WHERE id = sqlc.arg('id')::uuid
    AND workspace_id = sqlc.arg('workspace_id')::uuid
    AND status = 'executed';

-- name: RevertEffectEventUndo :execrows
-- Returns a claimed undo to executed when the inverse tool fails.
UPDATE effect_event SET status = 'executed'
WHERE id = sqlc.arg('id')::uuid
    AND workspace_id = sqlc.arg('workspace_id')::uuid
    AND status = 'undoing';

-- name: MarkEffectEventUndone :execrows
-- The second status move (durable spec 4.4): undoing to undone, once.
UPDATE effect_event SET status = 'undone'
WHERE id = sqlc.arg('id')::uuid
    AND workspace_id = sqlc.arg('workspace_id')::uuid
    AND status = 'undoing';
