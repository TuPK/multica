-- Stands in for the primary key on hub_item_version_blob (see 934).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS hub_item_version_blob_id_key
    ON hub_item_version_blob (id);
