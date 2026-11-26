-- At most one active delegation per person and kind. Built CONCURRENTLY, one statement per file.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS principal_delegation_active_uidx
    ON principal_delegation (workspace_id, member_id, decision_kind) WHERE disabled_at IS NULL;
