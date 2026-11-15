CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_seat_operation_uidx
    ON platform_seat_operation (workspace_id, operation_token);
