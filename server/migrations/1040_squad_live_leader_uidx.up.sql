CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS squad_live_leader_uidx ON squad (leader_id) WHERE archived_at IS NULL;
