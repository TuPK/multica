ALTER TABLE member
    DROP CONSTRAINT IF EXISTS member_role_check,
    ADD CONSTRAINT member_role_check
    CHECK (role IN ('owner', 'admin', 'member', 'viewer')) NOT VALID;

ALTER TABLE workspace_invitation
    DROP CONSTRAINT IF EXISTS workspace_invitation_role_check,
    ADD CONSTRAINT workspace_invitation_role_check
    CHECK (role IN ('admin', 'member', 'viewer')) NOT VALID;
