-- One envelope per squad (durable spec 3.4). Built concurrently; see 1080 for the table.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS squad_autonomy_envelope_key_uidx
    ON squad_autonomy_envelope (workspace_id, squad_id);
