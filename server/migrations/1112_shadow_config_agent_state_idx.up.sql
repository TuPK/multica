-- The primary's claim finds the seat's running config, and the status page lists them.
CREATE INDEX CONCURRENTLY IF NOT EXISTS shadow_config_agent_state_idx ON shadow_config (workspace_id, agent_id, state);
