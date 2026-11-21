-- Numbered 998 on purpose: the runner sorts file names as strings, so this has
-- to sort after 699_member_guest_role and 699_workspace_viewer_role, which a
-- four-digit name would not.
-- The fork's viewer role (699_workspace_viewer_role) and upstream's guest role
-- (699_member_guest_role) each replaced the role constraints with a list that
-- lacks the other's value, so whichever ran last would reject the other role.
-- This one allows both. NOT VALID keeps it catalog-only: widening a whitelist
-- cannot fail on existing rows.
ALTER TABLE member
    DROP CONSTRAINT IF EXISTS member_role_check,
    ADD CONSTRAINT member_role_check
    CHECK (role IN ('owner', 'admin', 'member', 'viewer', 'guest')) NOT VALID;

ALTER TABLE workspace_invitation
    DROP CONSTRAINT IF EXISTS workspace_invitation_role_check,
    ADD CONSTRAINT workspace_invitation_role_check
    CHECK (role IN ('admin', 'member', 'viewer', 'guest')) NOT VALID;
