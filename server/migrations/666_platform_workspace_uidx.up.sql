CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_workspace_uidx
    ON platform_workspace (workspace_id);
