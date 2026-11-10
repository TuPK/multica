-- The history of one document, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS knowledge_property_history_link_idx
    ON knowledge_property_history (workspace_id, link_id, created_at DESC);
