-- Pending intents that share a batch, for "approve all" (durable spec 5.2, 5.3).
CREATE INDEX CONCURRENTLY IF NOT EXISTS effect_intent_batch_idx
    ON effect_intent (workspace_id, batch_key) WHERE state = 'pending';
