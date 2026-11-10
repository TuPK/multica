ALTER TABLE hub_item
    DROP COLUMN IF EXISTS update_available,
    DROP COLUMN IF EXISTS remote_digest,
    DROP COLUMN IF EXISTS checked_at,
    DROP COLUMN IF EXISTS local_modified;
