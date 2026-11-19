-- One live request per workspace, kind and person: closed requests do not hold it.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_member_request_live_uidx
    ON platform_member_request (workspace_id, kind, email) WHERE state = 'pending';
