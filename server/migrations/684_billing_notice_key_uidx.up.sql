CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS billing_notice_key_uidx ON billing_notice (workspace_id, notice_key);
