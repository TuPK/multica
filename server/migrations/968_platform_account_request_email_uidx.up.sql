-- One live request per email: closed requests (completed, failed, canceled) do not hold it.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_account_request_email_uidx ON platform_account_request (email) WHERE state IN ('pending', 'provisioned');
