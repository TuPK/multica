ALTER TABLE workspace_invitation
    DROP COLUMN IF EXISTS guest_project_ids,
    DROP COLUMN IF EXISTS is_guest;
ALTER TABLE member DROP COLUMN IF EXISTS is_guest;
