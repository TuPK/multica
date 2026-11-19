-- An older platform reads every 'admin' as a platform-wide administrator, so a
-- tenant admin is revoked before system_admin goes back to admin.
UPDATE platform_staff SET revoked_at = now()
WHERE role = 'admin' AND tenant_id IS NOT NULL AND revoked_at IS NULL;
UPDATE platform_staff SET role = 'admin' WHERE role = 'system_admin';
