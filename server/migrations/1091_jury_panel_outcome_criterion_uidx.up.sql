-- One panel per criterion of an outcome. Opening a panel is idempotent, because the
-- check job runs again each minute, and this index serves the lookup by outcome too.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS jury_panel_outcome_criterion_uidx
    ON jury_panel (outcome_id, criterion_id);
