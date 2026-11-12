-- Backfill issue_workflow.squad_id for the SDLC and planning workflows that
-- already exist. The same links decide a workflow's squad in the code:
--   SDLC Auto*           its todo step hands to the agent "SDLC Developer"
--                        (sdlcWorkflowFor in service/initiative_build.go)
--   BMAD Planning Auto*  its blocked step hands to the agent "Blueprint Lead"
--                        (planningWorkflowFor in service/squad_planning_workflow.go)
-- The handler agent must be a member of exactly one squad in the workspace. A
-- workflow that fits two squads stays NULL, and its squad claims it from the
-- workflow settings, instead of being guessed here. Only rows still NULL are
-- set, so running the migration again changes nothing. Down: the column is
-- dropped by 973, so there is nothing to undo here.
WITH handler_links AS (
    SELECT w.id AS workflow_id,
           w.workspace_id,
           (step->'handler'->>'id')::uuid AS agent_id,
           CASE WHEN w.name LIKE 'SDLC Auto%' THEN 'SDLC Developer' ELSE 'Blueprint Lead' END AS handler_name
    FROM issue_workflow w
    CROSS JOIN LATERAL jsonb_array_elements(w.steps) AS step
    WHERE w.squad_id IS NULL
      AND NOT w.is_default
      AND step->'handler'->>'type' = 'agent'
      AND (
            (w.name LIKE 'SDLC Auto%' AND step->>'status_key' = 'todo')
         OR (w.name LIKE 'BMAD Planning Auto%' AND step->>'status_key' = 'blocked')
      )
),
squad_links AS (
    SELECT h.workflow_id, sm.squad_id
    FROM handler_links h
    JOIN agent a ON a.id = h.agent_id AND a.workspace_id = h.workspace_id AND a.name = h.handler_name
    JOIN squad_member sm ON sm.member_type = 'agent' AND sm.member_id = a.id
    JOIN squad s ON s.id = sm.squad_id AND s.workspace_id = h.workspace_id
)
UPDATE issue_workflow w
SET squad_id = one.squad_id
FROM (
    SELECT workflow_id, (ARRAY_AGG(DISTINCT squad_id))[1] AS squad_id
    FROM squad_links
    GROUP BY workflow_id
    HAVING COUNT(DISTINCT squad_id) = 1
) AS one
WHERE w.id = one.workflow_id
  AND w.squad_id IS NULL;
