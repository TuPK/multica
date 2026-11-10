-- The per-item history: one item's entries, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS hub_activity_workspace_ref_created_idx
    ON hub_activity (workspace_id, ref_id, created_at DESC);
