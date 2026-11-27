-- Store the provider execution result on settled effect_event rows (V2.7).
ALTER TABLE effect_event ADD COLUMN IF NOT EXISTS result JSONB;
