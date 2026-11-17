CREATE INDEX CONCURRENTLY IF NOT EXISTS project_member_user_idx ON project_member (user_id, project_id);
