-- The squad template a squad was created from (spec 7.5). Null for a squad made by hand
-- and for every squad made before this column. Set by the quickstart; not a foreign key.
ALTER TABLE squad ADD COLUMN IF NOT EXISTS template_key TEXT;
