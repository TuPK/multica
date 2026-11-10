-- Stands in for the primary key on hub_item (see 930).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS hub_item_id_key
    ON hub_item (id);
