CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_runtime_quickstart_request_runtime
    ON runtime_quickstart_request (runtime_id, status, created_at);
