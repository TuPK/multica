-- The idempotency key of a send: one attempt per channel version of an item.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS publish_attempt_idempotency_idx
    ON publish_attempt (workspace_id, item_id, channel_id, version);
