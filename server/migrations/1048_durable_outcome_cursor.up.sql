-- Read position of the outcome job over issue_status_transition (durable spec
-- 2.4, A2). One row per job name, moved by the job in the same transaction that
-- writes the outcomes it read, so a restart neither skips nor repeats a
-- transition. A cursor is (created_at, id), which is the order the job reads in.
-- No foreign keys (project rule).
CREATE TABLE durable_outcome_cursor (
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    transition_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
