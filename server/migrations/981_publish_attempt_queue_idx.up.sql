-- The publisher's queue: queued attempts, oldest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS publish_attempt_queued_idx
    ON publish_attempt (created_at)
    WHERE status = 'queued';
