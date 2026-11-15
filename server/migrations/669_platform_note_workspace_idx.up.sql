CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_note_workspace_idx
    ON platform_note (workspace_id, created_at DESC);
