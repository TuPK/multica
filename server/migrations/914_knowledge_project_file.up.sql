-- What the project docs ingest has written, one record per project and path.
-- Like knowledge_repo_file, it has no foreign keys: the ingest removes the
-- documents itself when a file leaves the project directory.
CREATE TABLE knowledge_project_file (
    workspace_id UUID NOT NULL,
    project_id UUID NOT NULL,
    path TEXT NOT NULL CHECK (char_length(path) BETWEEN 1 AND 1024),
    knowledge_link_id UUID,
    content_hash TEXT NOT NULL,
    archived_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
