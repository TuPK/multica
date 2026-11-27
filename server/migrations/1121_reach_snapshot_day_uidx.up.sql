-- One row per workspace, day and surface. A nightly run that repeats a day overwrites its
-- rows (durable spec 11.5, 3). Built concurrently; the migration runner drops an INVALID
-- index left by an interrupted build (concurrentIndexCleanups).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS reach_snapshot_day_uidx ON reach_snapshot (workspace_id, snapshot_date, surface);
