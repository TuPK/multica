CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_staff_user_idx
    ON platform_staff (user_id) WHERE revoked_at IS NULL;
