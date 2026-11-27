ALTER TABLE durable_outcome_cursor
    DROP COLUMN IF EXISTS pending_xmax,
    DROP COLUMN IF EXISTS pending_seq,
    DROP COLUMN IF EXISTS last_seq;
