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

-- name: InsertGatedEffectEvent :one
-- Evaluates task run class (shadow/verification/human) and workspace pause state inline
-- during the effect_event insert so Gate issues zero separate SELECT queries on the hot
-- path (durable spec 14.7) and never reads stale shadow or pause state.
INSERT INTO effect_event (workspace_id, task_id, agent_id, member_id, source, target,
    effect_class, reversibility, decision, status, summary, intent_id, app_tool_call_id)
SELECT
    sqlc.arg('workspace_id')::uuid,
    sqlc.narg('task_id')::uuid,
    sqlc.narg('agent_id')::uuid,
    sqlc.narg('member_id')::uuid,
    sqlc.arg('source')::text,
    sqlc.arg('target')::text,
    sqlc.arg('effect_class')::text,
    sqlc.arg('reversibility')::text,
    g.final_decision,
    CASE
        WHEN sqlc.arg('source')::text = 'declared' THEN 'declared'
        WHEN g.final_decision = 'block' THEN 'blocked'
        ELSE 'awaiting'
    END,
    sqlc.arg('summary')::text,
    sqlc.narg('intent_id')::uuid,
    sqlc.narg('app_tool_call_id')::uuid
FROM (
    SELECT
        CASE
            WHEN COALESCE(t.context->>'durable_kind', '') = 'shadow'
                 AND sqlc.arg('effect_class')::text <> 'read' THEN 'block'
            WHEN COALESCE(t.context->>'durable_kind', '') = 'verification'
                 AND sqlc.arg('effect_class')::text NOT IN ('read', 'write_internal') THEN 'block'
            WHEN w.company_state = 'paused'
                 AND sqlc.arg('effect_class')::text <> 'read'
                 AND sqlc.narg('task_id')::uuid IS NOT NULL
                 AND NOT (
                     COALESCE(t.context->>'durable_kind', '') NOT IN ('shadow', 'verification')
                     AND NOT (
                         COALESCE(r.source, '') = 'catchup'
                         OR (COALESCE(ap.assignee_type, '') = 'agent'
                             AND w.secretary_agent_id IS NOT NULL
                             AND COALESCE(ap.assignee_id = w.secretary_agent_id, false)
                             AND COALESCE(ap.title, '') IN ('Daily brief', 'Weekly review'))
                     )
                     AND (
                         COALESCE(r.source, '') = 'manual'
                         OR t.chat_session_id IS NOT NULL
                         OR COALESCE(t.originator_source, '') = 'direct_human'
                     )
                 ) THEN 'block'
            ELSE sqlc.arg('base_decision')::text
        END::text AS final_decision
    FROM workspace w
    LEFT JOIN agent_task_queue t
        ON sqlc.narg('task_id')::uuid IS NOT NULL
        AND t.id = sqlc.narg('task_id')::uuid
        -- agent_task_queue has no workspace column: a task counts only when its agent is in this workspace.
        AND EXISTS (SELECT 1 FROM agent ax WHERE ax.id = t.agent_id AND ax.workspace_id = w.id)
    LEFT JOIN agent ta
        ON ta.id = t.agent_id
        AND ta.workspace_id = w.id
    LEFT JOIN autopilot_run r
        ON r.id = t.autopilot_run_id
        AND ta.id IS NOT NULL
    LEFT JOIN autopilot ap
        ON ap.id = r.autopilot_id
        AND ap.workspace_id = w.id
    WHERE w.id = sqlc.arg('workspace_id')::uuid
      AND (sqlc.narg('task_id')::uuid IS NULL OR ta.id IS NOT NULL)
) g
RETURNING id, decision;

-- name: FinishEffectEvent :execrows
-- Moves an awaiting event to its outcome (executed|blocked|failed). undo, undo_until
-- and result are set only during this single transition when the forward call succeeded.
UPDATE effect_event SET
    status = sqlc.arg('status')::text,
    undo = COALESCE(sqlc.narg('undo')::jsonb, undo),
    undo_until = COALESCE(sqlc.narg('undo_until')::timestamptz, undo_until),
    result = COALESCE(sqlc.narg('result')::jsonb, result)
WHERE id = sqlc.arg('id')::uuid
    AND workspace_id = sqlc.arg('workspace_id')::uuid
    AND status = 'awaiting'
    AND sqlc.arg('status')::text IN ('executed', 'blocked', 'failed');

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
