-- A jury panel (durable spec 8.3, F7): the jurors who check one criterion of one
-- outcome, and where their votes stand. juror_agent_ids grows by one when a juror is
-- replaced. state moves open -> unanimous | split | escalated | timed_out, and split or
-- escalated -> resolved when a person decides. No foreign keys (project rule).
CREATE TABLE jury_panel (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    outcome_id UUID NOT NULL,
    criterion_id UUID,
    juror_agent_ids UUID[] NOT NULL,
    diversity TEXT NOT NULL CHECK (diversity IN ('provider', 'model_only', 'none')),
    state TEXT NOT NULL DEFAULT 'open'
        CHECK (state IN ('open', 'unanimous', 'split', 'escalated', 'resolved', 'timed_out')),
    verdict TEXT,
    resolved_by_type TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ
);
