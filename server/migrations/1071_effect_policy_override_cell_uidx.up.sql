-- One override per policy cell. Saves upsert on this index (durable spec 4.3).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS effect_policy_override_cell_uidx
    ON effect_policy_override (workspace_id, autonomy_level, effect_class, reversibility);
