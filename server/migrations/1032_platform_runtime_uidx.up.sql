-- One row per runtime. A concurrent build, so the migration runner runs it
-- outside a transaction (spec R6 section 8).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_runtime_uidx
    ON platform_runtime (runtime_id);
