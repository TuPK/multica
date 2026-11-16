CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_usage_daily_uidx ON platform_usage_daily (workspace_id, day);
