-- The items of one source, for source listings and removal.
CREATE INDEX CONCURRENTLY IF NOT EXISTS hub_item_workspace_source_idx
    ON hub_item (workspace_id, source_id);
