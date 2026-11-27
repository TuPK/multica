-- Numbered 998 on purpose: migrations run in leading-number order, so this file runs after
-- 630_project_visibility and 699_member_guest_role.
-- The fork modelled restricted projects with project.access and guests with
-- member.is_guest (1002, 1003). The workspace now uses upstream's model:
-- project.visibility = 'private' and member.role = 'guest'. Carry existing rows
-- over. The old columns stay in place and are no longer read or written.
-- A guest was a flag on a member or viewer; it becomes the guest role, which
-- loses the viewer part of that combination.
-- On a new database 1002 and 1003 run after this file, so their columns do not exist yet and there
-- is no data to carry over. Each backfill is guarded by its column and does nothing there.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'project' AND column_name = 'access') THEN
    UPDATE project SET visibility = 'private'
    WHERE access <> 'workspace' AND visibility = 'workspace';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'member' AND column_name = 'is_guest') THEN
    UPDATE member SET role = 'guest'
    WHERE is_guest AND role IN ('member', 'viewer');
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'workspace_invitation' AND column_name = 'is_guest') THEN
    UPDATE workspace_invitation
    SET role = 'guest', project_ids = guest_project_ids
    WHERE is_guest AND role IN ('member', 'viewer');
  END IF;
END $$;
