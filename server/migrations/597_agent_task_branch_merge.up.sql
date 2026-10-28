-- Records the outcome of a worktree run's opt-in merge into the user's
-- checked-out branch: 'merged', 'skipped' (a precondition refused it, with the
-- reason in branch_merge_error) or 'failed' (git refused it, typically a
-- conflict). Only completed runs are merged, so only completed rows populate it.
--
-- Nullable and unindexed on purpose: it is read alongside the task row that is
-- already being fetched by primary key.
ALTER TABLE agent_task_queue ADD COLUMN IF NOT EXISTS branch_merge_status TEXT;
ALTER TABLE agent_task_queue ADD COLUMN IF NOT EXISTS branch_merge_error TEXT;
