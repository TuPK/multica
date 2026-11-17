ALTER TABLE member ADD COLUMN IF NOT EXISTS is_guest boolean NOT NULL DEFAULT false;
ALTER TABLE workspace_invitation
    ADD COLUMN IF NOT EXISTS is_guest boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS guest_project_ids uuid[] NOT NULL DEFAULT '{}';
