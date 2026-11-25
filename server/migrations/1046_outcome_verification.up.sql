-- One check of an outcome (durable spec 2.3). Append-only: a new check is a new
-- row, and no query updates or deletes a row (TestOutcomeAppendOnlyVerifications).
-- No foreign keys (project rule).
CREATE TABLE outcome_verification (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    outcome_id UUID NOT NULL,
    criterion_id UUID,
    verifier_type TEXT NOT NULL CHECK (verifier_type IN ('system', 'agent', 'member', 'metric', 'jury')),
    verifier_id UUID,
    task_id UUID,
    result TEXT NOT NULL CHECK (result IN ('pass', 'partial', 'fail', 'unable')),
    severity TEXT NOT NULL DEFAULT 'none' CHECK (severity IN ('none', 'low', 'medium', 'high')),
    note TEXT NOT NULL DEFAULT '' CHECK (char_length(note) <= 4000),
    evidence JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
