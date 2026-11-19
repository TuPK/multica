-- platform_deletion_request: a staff member's request to delete a workspace or a
-- tenant now (spec R6 section 6). The platform writes the request and cancels or
-- retries it; the staff_deletion job owns state, error_code, attempts, updated_at
-- and completed_at. Runtime requests unregister the runtime through the owner's teardown.

-- name: ListPendingStaffDeletionIDs :many
SELECT id FROM platform_deletion_request
WHERE kind IN ('workspace', 'tenant', 'account', 'runtime')
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
  AND kind IN ('workspace', 'tenant', 'account', 'runtime')
  AND (state = 'pending' OR (state = 'running' AND updated_at < now() - interval '30 minutes'))
RETURNING *;

-- name: CompleteStaffDeletionRequest :exec
-- An account request drops the email it was filed with: the target label becomes
-- 'deleted user' (spec R6 section 7.4, phase 2 step 5).
UPDATE platform_deletion_request
SET state = 'completed', error_code = '', completed_at = now(), updated_at = now(),
    target_label = CASE WHEN kind = 'account' THEN 'deleted user' ELSE target_label END
WHERE id = $1 AND state = 'running';

-- name: ReleaseStaffDeletionClaim :exec
-- Returns a claimed request to pending without counting an attempt, for a request
-- that must wait for its child requests.
UPDATE platform_deletion_request
SET state = 'pending', updated_at = now()
WHERE id = $1 AND state = 'running';

-- name: FailStaffDeletionNow :exec
-- A failure no retry can fix: the request is failed at once with its error_code.
UPDATE platform_deletion_request
SET state = 'failed', error_code = @error_code, attempts = attempts + 1, updated_at = now()
WHERE id = @id AND state = 'running';

-- name: ListStaffDeletionChildStates :many
-- The states of the workspace requests an account request owns (spec R6 section 7.4).
SELECT state FROM platform_deletion_request WHERE parent_id = $1;

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
