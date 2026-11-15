CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_account_user_idx
    ON platform_account (user_id);
