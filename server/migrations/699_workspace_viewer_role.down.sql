-- Rolling back refuses while any row holds the viewer role: the validation of
-- the restored constraint fails. Move those rows to another role first (an
-- operator decision: a viewer must not be promoted by a rollback).
ALTER TABLE workspace_share_link DROP CONSTRAINT IF EXISTS workspace_share_link_role_check;
ALTER TABLE workspace_share_link ADD CONSTRAINT workspace_share_link_role_check
    CHECK (role IN ('admin', 'member')) NOT VALID;
ALTER TABLE workspace_share_link VALIDATE CONSTRAINT workspace_share_link_role_check;

ALTER TABLE workspace_invitation DROP CONSTRAINT IF EXISTS workspace_invitation_role_check;
ALTER TABLE workspace_invitation ADD CONSTRAINT workspace_invitation_role_check
    CHECK (role IN ('admin', 'member')) NOT VALID;
ALTER TABLE workspace_invitation VALIDATE CONSTRAINT workspace_invitation_role_check;

ALTER TABLE member DROP CONSTRAINT IF EXISTS member_role_check;
ALTER TABLE member ADD CONSTRAINT member_role_check
    CHECK (role IN ('owner', 'admin', 'member')) NOT VALID;
ALTER TABLE member VALIDATE CONSTRAINT member_role_check;
