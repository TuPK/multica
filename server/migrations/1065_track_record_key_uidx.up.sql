CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS track_record_key_uidx
    ON track_record (workspace_id, agent_id, work_class, window_days, model);
