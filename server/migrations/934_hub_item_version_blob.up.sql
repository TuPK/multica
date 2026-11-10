-- One immutable row per committed version of a hub item: the bundle that version
-- was committed or first served with (spec 7.3). Rows are only inserted, never
-- updated, so the history of bundles is append-only. No primary key and no
-- foreign keys, as for hub_item (930); the id index is created in 937, and the
-- (item_id, version, digest) uniqueness that makes a re-commit idempotent in 936.
CREATE TABLE IF NOT EXISTS hub_item_version_blob (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    item_id UUID NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    bundle_key TEXT NOT NULL,
    digest TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
