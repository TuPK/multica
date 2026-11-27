-- The lookup of an offer by its Hub org and id. Built concurrently; the migration runner drops
-- an INVALID index left by an interrupted build (concurrentIndexCleanups).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS seat_offer_workspace_id_uidx ON seat_offer (workspace_id, id);
