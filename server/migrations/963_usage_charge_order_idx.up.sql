CREATE INDEX CONCURRENTLY IF NOT EXISTS usage_charge_order_idx ON usage_charge (order_id) WHERE order_id IS NOT NULL;
