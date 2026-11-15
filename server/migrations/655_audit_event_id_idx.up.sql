CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS audit_event_id_idx
    ON audit_event (id);
