CREATE INDEX CONCURRENTLY IF NOT EXISTS issue_status_transition_issue_idx
    ON issue_status_transition (workspace_id, issue_id, created_at);
