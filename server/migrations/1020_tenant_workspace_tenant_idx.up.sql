CREATE INDEX CONCURRENTLY IF NOT EXISTS tenant_workspace_tenant_idx
    ON tenant_workspace (tenant_id);
