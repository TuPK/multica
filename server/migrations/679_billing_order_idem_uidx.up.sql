CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS billing_order_idem_uidx ON billing_order (workspace_id, idempotency_key) WHERE idempotency_key <> '';
