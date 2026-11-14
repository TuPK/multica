-- The installs of a squad, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_squad_pack_install_squad
    ON squad_pack_install (workspace_id, squad_id, installed_at DESC);
