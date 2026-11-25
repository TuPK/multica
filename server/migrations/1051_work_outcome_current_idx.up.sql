CREATE INDEX CONCURRENTLY IF NOT EXISTS work_outcome_current_idx
    ON work_outcome (workspace_id, issue_id) WHERE superseded_by IS NULL;
