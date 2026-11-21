-- Numbered 998 on purpose: the runner sorts file names as strings, so this has
-- to sort after 630_project_visibility and 699_member_guest_role, which a
-- four-digit name would not.
-- The fork modelled restricted projects with project.access and guests with
-- member.is_guest (1002, 1003). The workspace now uses upstream's model:
-- project.visibility = 'private' and member.role = 'guest'. Carry existing rows
-- over. The old columns stay in place and are no longer read or written.
-- A guest was a flag on a member or viewer; it becomes the guest role, which
-- loses the viewer part of that combination.
UPDATE project SET visibility = 'private'
WHERE access <> 'workspace' AND visibility = 'workspace';

UPDATE member SET role = 'guest'
WHERE is_guest AND role IN ('member', 'viewer');

UPDATE workspace_invitation
SET role = 'guest', project_ids = guest_project_ids
WHERE is_guest AND role IN ('member', 'viewer');
