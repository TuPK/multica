-- platform_deletion_request: a staff member's request to delete a workspace or a
-- tenant now (spec R6 section 6). The platform writes the request and cancels or
-- retries it; the staff_deletion job owns state, error_code, attempts, updated_at
-- and completed_at. Account and runtime requests belong to later slices.

-- name: ListPendingStaffDeletionIDs :many
SELECT id FROM platform_deletion_request
WHERE kind IN ('workspace', 'tenant')
  AND (state = 'pending' OR (state = 'running' AND updated_at < now() - interval '30 minutes'))
ORDER BY created_at, id
LIMIT $1;

-- name: ClaimStaffDeletionRequest :one
-- Moves a pending request to running. A running request whose claim is older
-- than the stale window was left by a run that stopped, so it is claimed again.
-- The row lock makes the claim exclusive: a second claimant finds no row.
UPDATE platform_deletion_request
SET state = 'running', updated_at = now()
WHERE id = $1
  AND kind IN ('workspace', 'tenant')
  AND (state = 'pending' OR (state = 'running' AND updated_at < now() - interval '30 minutes'))
RETURNING *;

-- name: CompleteStaffDeletionRequest :exec
UPDATE platform_deletion_request
SET state = 'completed', error_code = '', completed_at = now(), updated_at = now()
WHERE id = $1 AND state = 'running';

-- name: FailStaffDeletionAttempt :one
-- Counts a failed run. The last allowed attempt parks the request as failed for
-- staff to retry or cancel; an earlier failure returns it to pending for the next run.
UPDATE platform_deletion_request
SET attempts = attempts + 1,
    error_code = @error_code,
    state = CASE WHEN attempts + 1 >= @max_attempts::int THEN 'failed' ELSE 'pending' END,
    updated_at = now()
WHERE id = @id AND state = 'running'
RETURNING state;

-- name: HasLiveTenantDeletionRequest :one
-- A tenant deletion that has not completed holds the tenant: no workspace may be
-- created into it (spec R6 section 6.4).
SELECT EXISTS (
    SELECT 1 FROM platform_deletion_request
    WHERE kind = 'tenant' AND tenant_id = $1 AND state IN ('pending', 'running', 'failed')
);
