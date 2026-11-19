-- Tenant of the target of a staff deletion request (spec R6 section 6.3, D84). The
-- column is nullable and has no foreign key: a tenant may be deleted after its
-- request is closed, and the admin still lists the request by this id.
ALTER TABLE platform_deletion_request ADD COLUMN IF NOT EXISTS tenant_id uuid;
