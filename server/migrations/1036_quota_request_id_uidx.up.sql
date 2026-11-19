-- One row per id. A concurrent build, so the runner runs it outside a transaction.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS quota_request_id_uidx
    ON quota_request (id);
