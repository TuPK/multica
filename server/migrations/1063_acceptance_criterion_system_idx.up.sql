-- The hourly metric check and the artifact check read the open criteria of these
-- two kinds across workspaces (durable spec 2.5). Partial, so the index stays small.
CREATE INDEX CONCURRENTLY IF NOT EXISTS acceptance_criterion_system_check_idx
    ON acceptance_criterion (check_kind, workspace_id)
    WHERE archived_at IS NULL AND check_kind IN ('artifact', 'metric');
