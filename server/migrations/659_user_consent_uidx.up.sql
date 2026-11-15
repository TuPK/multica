CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS user_consent_uidx ON user_consent (user_id, document, version);
