CREATE INDEX CONCURRENTLY IF NOT EXISTS audit_event_tenant_created_idx
    ON audit_event (tenant_id, created_at DESC, id) WHERE tenant_id IS NOT NULL;
