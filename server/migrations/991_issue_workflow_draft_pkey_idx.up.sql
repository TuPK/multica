-- Backing index for issue_workflow_draft's primary key, attached in 992.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS issue_workflow_draft_pkey_uidx
    ON issue_workflow_draft (id);
