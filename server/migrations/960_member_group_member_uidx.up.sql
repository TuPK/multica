CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS member_group_member_uidx ON member_group_member (workspace_id, user_id, group_id);
