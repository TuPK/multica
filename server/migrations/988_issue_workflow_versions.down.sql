ALTER TABLE issue_workflow
    DROP COLUMN IF EXISTS published_by,
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS lineage_id;
