-- The config id is unique. No primary key: a primary key would build its index without
-- CONCURRENTLY, as the governance log notes.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS shadow_config_id_uidx ON shadow_config (id);
