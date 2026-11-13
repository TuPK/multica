-- Versions of a flow workflow (spec 2.1). A row with published_at is locked against edits;
-- lineage_id and version tie its versions together. No foreign key, no index here: 989 builds it.
ALTER TABLE issue_workflow
    ADD COLUMN IF NOT EXISTS lineage_id UUID,
    ADD COLUMN IF NOT EXISTS version INTEGER,
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS published_by UUID;
