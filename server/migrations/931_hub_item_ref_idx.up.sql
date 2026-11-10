-- One hub row per underlying skill or plugin package. Also the target of
-- UpsertHubItem (ON CONFLICT (workspace_id, kind, ref_id)).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS hub_item_workspace_kind_ref_key
    ON hub_item (workspace_id, kind, ref_id);
