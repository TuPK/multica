-- Dropping the constraint drops the index it took over, leaving 995's down
-- direction a no-op via IF EXISTS.
ALTER TABLE squad_pack_install DROP CONSTRAINT IF EXISTS squad_pack_install_pkey;
-- Attach the CONCURRENTLY-built unique index as the table's primary key.
ALTER TABLE squad_pack_install
    ADD CONSTRAINT squad_pack_install_pkey PRIMARY KEY USING INDEX squad_pack_install_pkey_uidx;
