CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS sso_domain_verified_domain_uidx ON sso_domain (domain) WHERE verified_at IS NOT NULL;
