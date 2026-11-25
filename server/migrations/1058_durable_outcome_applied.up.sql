-- Transitions the outcome job has applied (durable spec 2.4, A2). The job reads the
-- transitions after its cursor with an overlap, so a transition can be read more
-- than once. A row is inserted in the same transaction as the work it does, with
-- ON CONFLICT DO NOTHING; a transition whose row is not inserted is skipped. Rows
-- are never removed. No foreign keys (project rule).
CREATE TABLE durable_outcome_applied (
    transition_id UUID NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
