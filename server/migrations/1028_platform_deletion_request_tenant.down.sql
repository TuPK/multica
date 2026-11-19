-- Reverts 1028.
ALTER TABLE platform_deletion_request DROP COLUMN IF EXISTS tenant_id;
