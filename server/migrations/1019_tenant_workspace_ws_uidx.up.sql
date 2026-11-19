CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS tenant_workspace_ws_uidx
    ON tenant_workspace (workspace_id);
