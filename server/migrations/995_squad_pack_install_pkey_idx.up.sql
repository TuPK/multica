-- Backing index for squad_pack_install's primary key, attached in 996.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS squad_pack_install_pkey_uidx
    ON squad_pack_install (id);
