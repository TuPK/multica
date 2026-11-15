CREATE INDEX CONCURRENTLY IF NOT EXISTS billing_order_status_idx ON billing_order (status, created_at);
