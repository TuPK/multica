ALTER TABLE personal_access_token
    DROP COLUMN IF EXISTS read_only,
    DROP COLUMN IF EXISTS workspace_id;
