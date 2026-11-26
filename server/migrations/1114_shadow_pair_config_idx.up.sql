-- The report and the target count read a config's pairs.
CREATE INDEX CONCURRENTLY IF NOT EXISTS shadow_pair_config_idx ON shadow_pair (config_id);
