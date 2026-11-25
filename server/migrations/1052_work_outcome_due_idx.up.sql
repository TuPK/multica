CREATE INDEX CONCURRENTLY IF NOT EXISTS work_outcome_due_idx
    ON work_outcome (workspace_id, verdict, settle_at) WHERE settled_at IS NULL;
