-- The 24 hour timeout reads the open panels only (durable spec 8.3). Partial, so it
-- stays small once panels are decided.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jury_panel_open_created_idx
    ON jury_panel (created_at)
    WHERE state = 'open';
