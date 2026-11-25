-- The attention ledger (durable spec 7.3, F6). Every query filters by workspace_id; there
-- are no foreign keys. The budget is read and written through the service, which decides
-- each state.

-- name: InsertAttentionLedger :one
-- A question's row. A question already in the ledger keeps its row and state, so admitting
-- it again changes nothing and returns the state it has.
INSERT INTO attention_ledger (id, workspace_id, squad_id, subject_type, subject_id, reason, estimated_minutes, state, week_start)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.narg(squad_id), sqlc.arg(subject_type), sqlc.arg(subject_id),
    sqlc.arg(reason), sqlc.arg(estimated_minutes)::float8, sqlc.arg(state), sqlc.arg(week_start)
)
ON CONFLICT (subject_type, subject_id) DO UPDATE SET subject_type = EXCLUDED.subject_type
RETURNING state;

-- name: GetWorkspaceAttentionSettings :one
SELECT attention_minutes_per_week, attention_split FROM workspace
WHERE id = sqlc.arg(workspace_id);

-- name: SumSquadAttentionMinutes :one
-- The minutes a squad has spent in one week: delivered and answered questions. Queued and
-- delegated ones spend nothing.
SELECT COALESCE(SUM(estimated_minutes), 0)::float8 AS minutes FROM attention_ledger
WHERE workspace_id = sqlc.arg(workspace_id) AND squad_id = sqlc.arg(squad_id)
  AND week_start = sqlc.arg(week_start) AND state IN ('delivered', 'answered');

-- name: ListLiveSquadAttentionWeights :many
-- Each live squad with its active goals, the weights of the goals split.
SELECT s.id AS squad_id,
       (SELECT COUNT(*) FROM goal g
         WHERE g.workspace_id = s.workspace_id AND g.owner_type = 'squad'
           AND g.owner_id = s.id AND g.status = 'active')::bigint AS active_goals
FROM squad s
WHERE s.workspace_id = sqlc.arg(workspace_id) AND s.archived_at IS NULL
ORDER BY s.id;

-- name: MarkAttentionAnswered :execrows
-- The person decided the question. A question with no row, or one already answered, is
-- left as it is.
UPDATE attention_ledger
SET state = 'answered', answered_at = now(), actual_seconds = sqlc.narg(actual_seconds)::integer
WHERE subject_type = sqlc.arg(subject_type) AND subject_id = sqlc.arg(subject_id)
  AND state IN ('queued', 'delivered');

-- name: ListQueuedAttentionAsks :many
-- The questions waiting for the budget, in the order the brief shows them: the reasons that
-- block a decision first, then the oldest.
SELECT subject_type, subject_id, squad_id, reason, estimated_minutes::float8 AS estimated_minutes, created_at
FROM attention_ledger
WHERE workspace_id = sqlc.arg(workspace_id) AND state = 'queued'
ORDER BY CASE reason
           WHEN 'irreversible' THEN 0
           WHEN 'judgment' THEN 1
           WHEN 'policy' THEN 2
           WHEN 'verification' THEN 3
           WHEN 'missing_access' THEN 4
           ELSE 5
         END,
         created_at, subject_id
LIMIT sqlc.arg(row_limit);

-- name: GetAgentHomeSquadID :one
-- The squad an agent works for, which is the squad an agent's question is charged to.
SELECT home_squad_id FROM agent
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);
