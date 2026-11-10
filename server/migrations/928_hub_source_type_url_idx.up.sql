-- One source per type and URL in a workspace. Also the target of the import
-- commit's upsert (ON CONFLICT (workspace_id, type, url)).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS hub_source_workspace_type_url_key
    ON hub_source (workspace_id, type, url);
