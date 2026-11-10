-- A place skills and plugins are pulled from (spec 7.1). One row per source.
-- No primary key and no foreign keys: an implicit primary-key index would not be
-- built CONCURRENTLY, so the id index is created in its own migration (928). The
-- workspace link is checked in the application, which also removes its sources.
CREATE TABLE IF NOT EXISTS hub_source (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
    type TEXT NOT NULL CHECK (type IN ('github', 'skills_sh', 'clawhub', 'url_zip', 'registry', 'upload')),
    url TEXT NOT NULL DEFAULT '' CHECK (char_length(url) <= 2048),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    auto_sync_interval_sec INT NOT NULL DEFAULT 0 CHECK (auto_sync_interval_sec >= 0),
    last_sync_at TIMESTAMPTZ,
    last_sync_status TEXT NOT NULL DEFAULT '',
    last_sync_error TEXT NOT NULL DEFAULT '',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
