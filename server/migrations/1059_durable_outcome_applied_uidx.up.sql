CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS durable_outcome_applied_uidx
    ON durable_outcome_applied (transition_id);
