-- Typed properties on a knowledge document (spec 7.1). The fields a kind declares
-- (knowledge_kind.fields, migration 923) are validated on write; the values live here,
-- so changing a property does not re-index the document. Empty for every document a
-- kind without fields wrote.
ALTER TABLE knowledge_link ADD COLUMN IF NOT EXISTS properties JSONB NOT NULL DEFAULT '{}'::jsonb;
