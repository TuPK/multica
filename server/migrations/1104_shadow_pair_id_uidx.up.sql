-- The pair id is unique. No primary key, for the same reason as shadow_config_id_uidx.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS shadow_pair_id_uidx ON shadow_pair (id);
