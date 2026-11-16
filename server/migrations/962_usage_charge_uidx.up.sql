CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS usage_charge_uidx ON usage_charge (workspace_id, day, meter);
