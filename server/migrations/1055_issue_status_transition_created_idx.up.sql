CREATE INDEX CONCURRENTLY IF NOT EXISTS issue_status_transition_created_idx
    ON issue_status_transition (created_at, id);
