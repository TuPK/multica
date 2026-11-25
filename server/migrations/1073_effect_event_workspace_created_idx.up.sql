-- The governance timeline, newest first (durable spec 4.7).
CREATE INDEX CONCURRENTLY IF NOT EXISTS effect_event_workspace_created_idx
    ON effect_event (workspace_id, created_at DESC);
