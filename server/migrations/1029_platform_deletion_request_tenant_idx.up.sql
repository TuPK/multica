-- Lists a tenant's deletion requests for its admin (spec R6 section 6.5).
CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_deletion_request_tenant_idx
    ON platform_deletion_request (tenant_id, created_at DESC);
