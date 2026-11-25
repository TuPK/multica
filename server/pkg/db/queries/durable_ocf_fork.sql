-- Open Company Format export (durable E2). Read-only: the history parts of a
-- package read these rows. Ordering is stable so a repeated export is the same.

-- name: ListSettledWorkOutcomesForOCF :many
-- Settled outcomes, superseded ones included (the package marks them).
SELECT * FROM work_outcome
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND settled_at IS NOT NULL
  AND verdict <> 'pending'
ORDER BY settled_at, id;

-- name: ListTrackRecordsForOCF :many
-- Every track record row: the per-model rows and the all-model totals.
SELECT * FROM track_record
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
ORDER BY agent_id, work_class, window_days, model;

-- name: ListGovernanceEventsForOCF :many
SELECT * FROM governance_event
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
ORDER BY created_at, id;
