-- Replaced by project.squad_ids (migration 971). The squad list is the only record.
ALTER TABLE project DROP COLUMN IF EXISTS squad_id;
