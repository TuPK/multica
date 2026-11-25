-- Work outcome record (durable spec 2.3, D4 to D6). One row per entry of an issue
-- into done. The outcome job writes it from issue_status_transition, so no status
-- writer can drop an outcome by forgetting it.
--
-- No foreign keys (project rule): issue, transition and agent relations are
-- checked in application code.
CREATE TABLE work_outcome (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    done_transition_id UUID NOT NULL,
    agent_id UUID,
    member_id UUID,
    squad_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    work_class TEXT NOT NULL CHECK (char_length(work_class) <= 200),
    verification_mode TEXT NOT NULL CHECK (verification_mode IN ('full', 'sampled', 'spot')),
    sampled BOOLEAN NOT NULL,
    verdict TEXT NOT NULL DEFAULT 'pending'
        CHECK (verdict IN ('pending', 'pass', 'partial', 'fail', 'unverified')),
    severity TEXT NOT NULL DEFAULT 'none' CHECK (severity IN ('none', 'low', 'medium', 'high')),
    fail_cause TEXT NOT NULL DEFAULT ''
        CHECK (fail_cause IN ('', 'criteria', 'reopened', 'reverted', 'metric', 'jury', 'member')),
    criteria_results JSONB NOT NULL DEFAULT '[]',
    evidence JSONB NOT NULL DEFAULT '[]',
    cost_micros BIGINT NOT NULL DEFAULT 0,
    verification_cost_micros BIGINT NOT NULL DEFAULT 0,
    settle_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    superseded_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
