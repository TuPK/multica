CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_staff_tenant_idx
    ON platform_staff (tenant_id) WHERE revoked_at IS NULL AND tenant_id IS NOT NULL;
