CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS einvoice_issuance_order_uidx ON einvoice_issuance (order_id, revision);
