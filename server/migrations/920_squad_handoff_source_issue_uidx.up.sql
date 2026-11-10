-- A request is handed off once: a second request finds the first row.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS squad_handoff_source_issue_uidx
    ON squad_handoff (workspace_id, source_issue_id);
