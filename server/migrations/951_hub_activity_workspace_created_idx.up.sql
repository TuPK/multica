-- The activity feed: a workspace's entries, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS hub_activity_workspace_created_idx
    ON hub_activity (workspace_id, created_at DESC);
