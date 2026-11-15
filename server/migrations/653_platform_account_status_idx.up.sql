CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_account_status_idx
    ON platform_account (status) WHERE status <> 'active';
