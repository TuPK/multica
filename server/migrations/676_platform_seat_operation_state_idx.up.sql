CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_seat_operation_state_idx
    ON platform_seat_operation (workspace_id, state);
