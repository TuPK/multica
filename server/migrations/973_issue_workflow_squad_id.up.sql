-- The squad whose library a workflow belongs to. A squad owns several workflows
-- and one of them is its active workflow (squad.active_workflow_id, migration
-- 974). NULL means the workflow is workspace-level: a project's own workflow,
-- or one a squad has not adopted yet. Nullable with no default, so adding it is
-- metadata-only. No foreign key, by project rule: the squad handler checks it.
ALTER TABLE issue_workflow ADD COLUMN IF NOT EXISTS squad_id UUID;
