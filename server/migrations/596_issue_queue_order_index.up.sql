CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_queue_order
    ON issue (workspace_id, project_id, queue_order)
    WHERE queue_order IS NOT NULL;
