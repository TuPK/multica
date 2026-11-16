CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS workspace_export_running_uidx ON workspace_export (workspace_id) WHERE status IN ('queued','running');
