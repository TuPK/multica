-- One live request per target: a request that is completed or canceled does not hold it.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_deletion_request_live_uidx
    ON platform_deletion_request (kind, target_id)
    WHERE state IN ('awaiting_approval', 'pending', 'running', 'failed');
