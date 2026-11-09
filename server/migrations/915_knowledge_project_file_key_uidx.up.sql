-- One record per project and path. The ingest updates it in place.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS knowledge_project_file_key_uidx
    ON knowledge_project_file (workspace_id, project_id, path);
