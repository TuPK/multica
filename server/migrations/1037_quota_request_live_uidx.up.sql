-- At most one pending request per workspace and gate. Closed requests do not hold it.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS quota_request_live_uidx
    ON quota_request (workspace_id, gate) WHERE state = 'pending';
