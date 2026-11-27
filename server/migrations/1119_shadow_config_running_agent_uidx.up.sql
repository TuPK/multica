-- A seat has at most one running shadow run at a time.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS shadow_config_running_agent_uidx
    ON shadow_config (workspace_id, agent_id)
    WHERE state = 'running';
