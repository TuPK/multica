CREATE INDEX CONCURRENTLY IF NOT EXISTS workspace_subscription_status_idx
    ON workspace_subscription (status, current_period_end);
