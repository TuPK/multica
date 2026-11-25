-- The entry id is unique. No primary key: a primary key would build its index without
-- CONCURRENTLY, as the governance log notes.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS seat_dossier_entry_id_uidx
    ON seat_dossier_entry (id);
