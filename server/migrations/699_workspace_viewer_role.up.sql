-- Viewer role (platform management spec 6.6). A viewer reads what a member
-- reads and writes nothing except personal state; the refusal lives in
-- workspaceguard, the role here only admits the value. Each constraint is
-- dropped and re-added NOT VALID, then validated, so the table is not held
-- under an exclusive lock while the existing rows are checked. No foreign key
-- and no index.
ALTER TABLE member DROP CONSTRAINT IF EXISTS member_role_check;
ALTER TABLE member ADD CONSTRAINT member_role_check
    CHECK (role IN ('owner', 'admin', 'member', 'viewer')) NOT VALID;
ALTER TABLE member VALIDATE CONSTRAINT member_role_check;

ALTER TABLE workspace_invitation DROP CONSTRAINT IF EXISTS workspace_invitation_role_check;
ALTER TABLE workspace_invitation ADD CONSTRAINT workspace_invitation_role_check
    CHECK (role IN ('admin', 'member', 'viewer')) NOT VALID;
ALTER TABLE workspace_invitation VALIDATE CONSTRAINT workspace_invitation_role_check;

ALTER TABLE workspace_share_link DROP CONSTRAINT IF EXISTS workspace_share_link_role_check;
ALTER TABLE workspace_share_link ADD CONSTRAINT workspace_share_link_role_check
    CHECK (role IN ('admin', 'member', 'viewer')) NOT VALID;
ALTER TABLE workspace_share_link VALIDATE CONSTRAINT workspace_share_link_role_check;
