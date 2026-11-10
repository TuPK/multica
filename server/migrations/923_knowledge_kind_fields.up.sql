-- The typed fields a knowledge kind declares (spec 6.2): a JSON array of
-- {key, label, type, options, required}. Empty for every kind without fields.
ALTER TABLE knowledge_kind ADD COLUMN IF NOT EXISTS fields JSONB NOT NULL DEFAULT '[]'::jsonb;
