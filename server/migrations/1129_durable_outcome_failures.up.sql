ALTER TABLE durable_outcome_applied
    ADD COLUMN status TEXT NOT NULL DEFAULT 'applied' CHECK (status IN ('applied', 'failed', 'dead_letter')),
    ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
