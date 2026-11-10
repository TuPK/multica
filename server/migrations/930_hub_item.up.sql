-- The hub's read model: one row per skill or plugin package it knows about (spec 7.2).
-- ref_id is skill.id for kind 'skill' and plugin_package.id for kind 'plugin'.
-- No primary key and no foreign keys, for the same reason as hub_source (927).
CREATE TABLE IF NOT EXISTS hub_item (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('skill', 'plugin')),
    ref_id UUID NOT NULL,
    source_id UUID,
    source_ref TEXT NOT NULL DEFAULT '',
    source_digest TEXT NOT NULL DEFAULT '',
    tracking TEXT NOT NULL DEFAULT 'manual' CHECK (tracking IN ('tracking', 'forked', 'manual')),
    latest_version TEXT NOT NULL DEFAULT '',
    bundle_key TEXT NOT NULL DEFAULT '',
    bundle_digest TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    file_count INT NOT NULL DEFAULT 0 CHECK (file_count >= 0),
    scan_status TEXT NOT NULL DEFAULT '',
    scan_report JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
