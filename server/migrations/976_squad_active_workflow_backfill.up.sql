-- Backfill squad.active_workflow_id with the workflow each squad already uses.
-- Before this model a squad's projects took its SDLC or planning workflow
-- through project.workflow_id, so a squad's active workflow is the one of its
-- library (975) that the most projects assigned to the squad use. Ties go to
-- the oldest workflow. A squad with no such workflow stays NULL and keeps
-- routing through its projects. Only NULL values are set, so running the
-- migration again changes nothing. Down: the column is dropped by 974.
WITH used AS (
    SELECT DISTINCT ON (w.squad_id) w.squad_id, w.id AS workflow_id
    FROM issue_workflow w
    LEFT JOIN project p
      ON p.workspace_id = w.workspace_id
     AND p.workflow_id = w.id
     AND p.squad_ids @> ARRAY[w.squad_id]
    WHERE w.squad_id IS NOT NULL
    GROUP BY w.squad_id, w.id, w.created_at
    ORDER BY w.squad_id, COUNT(p.id) DESC, w.created_at, w.id
)
UPDATE squad s
SET active_workflow_id = used.workflow_id
FROM used
WHERE s.id = used.squad_id
  AND s.active_workflow_id IS NULL;
