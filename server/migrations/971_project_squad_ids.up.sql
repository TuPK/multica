-- The squads a project is assigned to. Several rooms can work one project at
-- once; a squad still holds at most one open project (checked by the handler).
-- Copies the single project.squad_id; migration 972 drops that column.
ALTER TABLE project ADD COLUMN IF NOT EXISTS squad_ids UUID[] NOT NULL DEFAULT '{}';
UPDATE project SET squad_ids = ARRAY[squad_id] WHERE squad_id IS NOT NULL AND squad_ids = '{}';
