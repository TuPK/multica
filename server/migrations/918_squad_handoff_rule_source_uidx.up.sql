-- One rule per source squad.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS squad_handoff_rule_source_uidx
    ON squad_handoff_rule (workspace_id, source_squad_id);
