CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS durable_outcome_cursor_name_uidx
    ON durable_outcome_cursor (name);
