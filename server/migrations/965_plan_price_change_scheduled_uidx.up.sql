CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS plan_price_change_scheduled_uidx ON plan_price_change (plan_key) WHERE state = 'scheduled';
