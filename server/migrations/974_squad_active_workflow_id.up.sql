-- The workflow a squad uses now. Issues assigned to the squad, or to an agent
-- whose home squad it is, take this workflow instead of their project's. NULL
-- means the squad has none yet and its issues keep the project's workflow.
-- Nullable with no default, so adding it is metadata-only. No foreign key, by
-- project rule: switching or deleting a workflow checks it in the handler.
ALTER TABLE squad ADD COLUMN IF NOT EXISTS active_workflow_id UUID;
