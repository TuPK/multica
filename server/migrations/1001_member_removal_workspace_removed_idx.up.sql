CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_member_removal_workspace_removed ON member_removal (workspace_id, removed_at);
