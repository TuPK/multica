-- name: CreateRuntimeQuickstartRequest :one
INSERT INTO runtime_quickstart_request (runtime_id, workspace_id, initiator_user_id, status)
VALUES ($1, $2, $3, 'pending')
RETURNING *;

-- name: GetRuntimeQuickstartRequest :one
SELECT * FROM runtime_quickstart_request
WHERE id = $1;

-- name: GetLiveRuntimeQuickstartRequest :one
-- The newest request that is still live: pending for at most two minutes, or
-- running for at most ten. A new request is refused while one is live. The
-- windows match the timeouts the handler applies when it reports a status.
SELECT * FROM runtime_quickstart_request
WHERE runtime_id = $1
  AND (
    (status = 'pending' AND created_at > now() - interval '2 minutes')
    OR (status = 'running' AND created_at > now() - interval '10 minutes')
  )
ORDER BY created_at DESC
LIMIT 1;

-- name: ClaimPendingRuntimeQuickstartRequest :one
-- Hands the oldest fresh pending request to the daemon. A request nobody
-- claimed within two minutes is left alone; the UI reports it as timed out.
UPDATE runtime_quickstart_request
SET status = 'running', updated_at = now()
WHERE id = (
    SELECT q.id FROM runtime_quickstart_request q
    WHERE q.runtime_id = $1
      AND q.status = 'pending'
      AND q.created_at > now() - interval '2 minutes'
    ORDER BY q.created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: CompleteRuntimeQuickstartRequest :one
UPDATE runtime_quickstart_request
SET status = 'completed', steps = $2, updated_at = now()
WHERE id = $1 AND status IN ('pending', 'running')
RETURNING *;

-- name: FailRuntimeQuickstartRequest :one
UPDATE runtime_quickstart_request
SET status = 'failed', steps = $2, error = $3, updated_at = now()
WHERE id = $1 AND status IN ('pending', 'running')
RETURNING *;
