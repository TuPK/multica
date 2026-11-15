CREATE INDEX CONCURRENTLY IF NOT EXISTS audit_event_actor_created_idx
    ON audit_event (actor_id, created_at DESC);
