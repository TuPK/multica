CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS workspace_subscription_uidx
    ON workspace_subscription (workspace_id);
