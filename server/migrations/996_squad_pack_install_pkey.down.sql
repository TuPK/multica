-- Dropping the constraint drops the index it took over, leaving 995's down
-- direction a no-op via IF EXISTS.
ALTER TABLE squad_pack_install DROP CONSTRAINT IF EXISTS squad_pack_install_pkey;
