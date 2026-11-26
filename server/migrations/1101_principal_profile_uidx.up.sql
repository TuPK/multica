-- One profile per member of a workspace. Built CONCURRENTLY, one statement per file (project rule).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS principal_profile_member_uidx
    ON principal_profile (workspace_id, member_id);
