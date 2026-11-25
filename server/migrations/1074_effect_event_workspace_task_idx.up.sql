-- A run's effects, for the run detail (durable spec 4.7).
CREATE INDEX CONCURRENTLY IF NOT EXISTS effect_event_workspace_task_idx
    ON effect_event (workspace_id, task_id);
