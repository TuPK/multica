-- Find the handoff that created a request.
CREATE INDEX CONCURRENTLY IF NOT EXISTS squad_handoff_target_issue_idx
    ON squad_handoff (workspace_id, target_issue_id) WHERE target_issue_id IS NOT NULL;
