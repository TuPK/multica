CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS payment_notice_event_uidx ON payment_notice (provider, provider_event_id);
