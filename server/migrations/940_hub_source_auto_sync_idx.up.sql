-- The auto-sync sweep reads only enabled sources with an interval, oldest sync first.
-- A partial index keeps that scan small when most sources are manual.
CREATE INDEX CONCURRENTLY IF NOT EXISTS hub_source_auto_sync_due_idx
    ON hub_source (last_sync_at NULLS FIRST)
    WHERE enabled AND auto_sync_interval_sec > 0;
