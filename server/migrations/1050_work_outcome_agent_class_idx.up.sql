CREATE INDEX CONCURRENTLY IF NOT EXISTS work_outcome_agent_class_settled_idx
    ON work_outcome (workspace_id, agent_id, work_class, settled_at);
