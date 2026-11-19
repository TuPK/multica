-- At most one live hold per scope. A released hold stays as history.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS legal_hold_live_uidx
    ON legal_hold (scope_kind, scope_id) WHERE released_at IS NULL;
