-- Keep only the newest running shadow_config per (workspace_id, agent_id) before
-- enforcing uniqueness with a partial unique index; older duplicates move to 'stopped'.
UPDATE shadow_config sc
SET state = 'stopped',
    completed_at = COALESCE(completed_at, now())
WHERE sc.state = 'running'
  AND EXISTS (
      SELECT 1
      FROM shadow_config newer
      WHERE newer.workspace_id = sc.workspace_id
        AND newer.agent_id = sc.agent_id
        AND newer.state = 'running'
        AND (newer.created_at > sc.created_at
             OR (newer.created_at = sc.created_at AND newer.id > sc.id))
  );
