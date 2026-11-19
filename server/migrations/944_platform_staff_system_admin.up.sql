-- Every administrator before the tenant model governed the whole platform, which
-- is what system_admin now means. An admin with a tenant_id is a tenant admin
-- and stays. Running it again changes nothing.
UPDATE platform_staff SET role = 'system_admin' WHERE role = 'admin' AND tenant_id IS NULL;
