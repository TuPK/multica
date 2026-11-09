-- The folder a document is filed under inside its knowledge base, such as
-- docs/planning-artifacts for a project file. Empty means the base's top level.
ALTER TABLE knowledge_link ADD COLUMN folder TEXT NOT NULL DEFAULT '';
