-- Records the principal-learning routine task that consumed a resolved decision,
-- so re-running the job never enqueues a second task for the same decisions.
ALTER TABLE decision_prediction
    ADD COLUMN IF NOT EXISTS learning_task_id UUID;
