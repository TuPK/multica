ALTER TABLE project ADD COLUMN IF NOT EXISTS squad_id UUID;
UPDATE project SET squad_id = squad_ids[1] WHERE cardinality(squad_ids) > 0;
