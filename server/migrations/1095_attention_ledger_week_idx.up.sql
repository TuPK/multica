-- The budget read: a squad's minutes in one week, and the queued asks of a workspace
-- for the digest.
CREATE INDEX CONCURRENTLY IF NOT EXISTS attention_ledger_week_idx
    ON attention_ledger (workspace_id, week_start, squad_id);
