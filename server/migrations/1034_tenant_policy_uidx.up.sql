-- One policy row per tenant. A concurrent build, so the runner runs it outside a transaction.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS tenant_policy_uidx
    ON tenant_policy (tenant_id);
