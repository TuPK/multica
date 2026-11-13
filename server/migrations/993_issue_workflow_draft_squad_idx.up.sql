-- A squad's drafts, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_workflow_draft_squad
    ON issue_workflow_draft (workspace_id, squad_id, updated_at DESC);
