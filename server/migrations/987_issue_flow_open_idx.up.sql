-- The runner's scan: requests that run on a flow and are not closed. The engine
-- leaves closed_at out of flow until it closes the request.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_flow_open
    ON issue (workspace_id, id)
    WHERE flow IS NOT NULL AND NOT (flow ? 'closed_at');
