CREATE INDEX CONCURRENTLY IF NOT EXISTS billing_order_workspace_idx ON billing_order (workspace_id, created_at DESC);
