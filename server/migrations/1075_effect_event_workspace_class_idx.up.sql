-- Filters by effect class, and the send count the L3 daily cap reads (durable spec 4.3).
CREATE INDEX CONCURRENTLY IF NOT EXISTS effect_event_workspace_class_idx
    ON effect_event (workspace_id, effect_class, created_at);
