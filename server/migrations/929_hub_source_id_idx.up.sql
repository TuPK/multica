-- Stands in for the primary key on hub_source (see 927).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS hub_source_id_key
    ON hub_source (id);
