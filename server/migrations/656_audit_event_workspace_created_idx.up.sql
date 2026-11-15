CREATE INDEX CONCURRENTLY IF NOT EXISTS audit_event_workspace_created_idx
    ON audit_event (workspace_id, created_at DESC, id);
