-- Update-check state per hub item. Phase 5 (source sync) writes these; Phase 4
-- only reads them, and they hold their defaults until then.
ALTER TABLE hub_item
    ADD COLUMN IF NOT EXISTS update_available BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS remote_digest TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS checked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS local_modified BOOLEAN NOT NULL DEFAULT FALSE;
