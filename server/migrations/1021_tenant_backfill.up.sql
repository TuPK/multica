-- Gives every workspace without a tenant a tenant of its own, keyed by the
-- workspace id (spec R6 section 5.4). The same statements run again as
-- `platform-backfill tenants`, so they must be safe to repeat and to run at the
-- same time: the unique indexes of 1017 and 1019 absorb the conflicts. Only
-- tables created before this file in sort order may appear here.
INSERT INTO tenant (id, name, created_at, updated_at)
SELECT w.id, w.name, w.created_at, w.created_at
FROM workspace w
WHERE NOT EXISTS (SELECT 1 FROM tenant_workspace tw WHERE tw.workspace_id = w.id)
ORDER BY w.id
ON CONFLICT (id) DO NOTHING;

INSERT INTO tenant_workspace (workspace_id, tenant_id, created_at)
SELECT w.id, w.id, w.created_at
FROM workspace w
WHERE NOT EXISTS (SELECT 1 FROM tenant_workspace tw WHERE tw.workspace_id = w.id)
ORDER BY w.id
ON CONFLICT (workspace_id) DO NOTHING;
