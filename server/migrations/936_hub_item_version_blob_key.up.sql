-- One row per (item, version, digest). A re-commit of the same bundle conflicts
-- here and is skipped, so committing twice writes one version row.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS hub_item_version_blob_item_version_digest_key
    ON hub_item_version_blob (item_id, version, digest);
