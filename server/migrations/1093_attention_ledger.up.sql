-- The attention ledger (durable spec 7.3, F6): one row per question put to a person
-- to decide, with its estimated minutes and where it stands. No foreign keys (project
-- rule); the subject is the proposal, intent, outcome or inquiry the row names.
CREATE TABLE attention_ledger (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID,
    squad_id UUID,
    subject_type TEXT NOT NULL
        CHECK (subject_type IN ('proposal', 'effect_intent', 'outcome_check', 'app_action_request', 'inquiry')),
    subject_id UUID NOT NULL,
    reason TEXT NOT NULL
        CHECK (reason IN ('judgment', 'irreversible', 'missing_access', 'missing_info', 'policy', 'verification')),
    estimated_minutes NUMERIC NOT NULL DEFAULT 0,
    actual_seconds INTEGER,
    state TEXT NOT NULL
        CHECK (state IN ('queued', 'delivered', 'answered', 'expired', 'delegated')),
    week_start DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at TIMESTAMPTZ
);

-- The workspace settings of the budget (spec 7.3). Plain columns rather than keys in
-- workspace.settings, for the reason migration 842 gives. The CHECKs are added NOT
-- VALID and validated after, as in 842.
ALTER TABLE workspace ADD COLUMN IF NOT EXISTS attention_minutes_per_week INTEGER NOT NULL DEFAULT 60;
ALTER TABLE workspace ADD COLUMN IF NOT EXISTS attention_split TEXT NOT NULL DEFAULT 'equal';
ALTER TABLE workspace DROP CONSTRAINT IF EXISTS workspace_attention_minutes_per_week_check;
ALTER TABLE workspace ADD CONSTRAINT workspace_attention_minutes_per_week_check
    CHECK (attention_minutes_per_week >= 0) NOT VALID;
ALTER TABLE workspace VALIDATE CONSTRAINT workspace_attention_minutes_per_week_check;
ALTER TABLE workspace DROP CONSTRAINT IF EXISTS workspace_attention_split_check;
ALTER TABLE workspace ADD CONSTRAINT workspace_attention_split_check
    CHECK (attention_split IN ('equal', 'goals')) NOT VALID;
ALTER TABLE workspace VALIDATE CONSTRAINT workspace_attention_split_check;
