CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_workspace_status_idx
    ON platform_workspace (status) WHERE status <> 'active';
