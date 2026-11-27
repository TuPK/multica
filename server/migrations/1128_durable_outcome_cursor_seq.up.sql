-- Track the safe contiguous sequence cursor and pending snapshot barrier on
-- durable_outcome_cursor so late-committing transactions are never skipped (V2.2).
ALTER TABLE durable_outcome_cursor
    ADD COLUMN IF NOT EXISTS last_seq BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pending_seq BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pending_xmax BIGINT NOT NULL DEFAULT 0;
