CREATE INDEX CONCURRENTLY IF NOT EXISTS issue_status_transition_to_status_idx
    ON issue_status_transition (workspace_id, to_status_key, created_at);
