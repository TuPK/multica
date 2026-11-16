CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS member_group_name_uidx ON member_group (workspace_id, lower(name));
