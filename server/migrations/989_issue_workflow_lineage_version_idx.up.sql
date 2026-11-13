-- One version number per lineage.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_workflow_lineage_version
    ON issue_workflow (lineage_id, version)
    WHERE lineage_id IS NOT NULL;
