-- The intents page, by state and age (durable spec 5.2).
CREATE INDEX CONCURRENTLY IF NOT EXISTS effect_intent_state_idx
    ON effect_intent (workspace_id, state, created_at);
