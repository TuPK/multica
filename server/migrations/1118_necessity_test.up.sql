-- A necessity test asks whether one piece of scaffolding (a skill, F9) still earns its place:
-- a shadow run of the seat without that piece, compared blind (durable spec 10.2). No foreign
-- keys: the seat, the shadow run and the proposal are checked in application code.
CREATE TABLE IF NOT EXISTS necessity_test (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('skill', 'instruction_block', 'flow_stage')),
    subject_ref TEXT NOT NULL,
    agent_id UUID NOT NULL,
    shadow_config_id UUID,
    margin NUMERIC NOT NULL DEFAULT 0.05,
    min_pairs INTEGER NOT NULL DEFAULT 20,
    state TEXT NOT NULL DEFAULT 'running'
        CHECK (state IN ('running', 'not_needed', 'needed', 'inconclusive', 'stopped')),
    result JSONB NOT NULL DEFAULT '{}',
    proposal_id UUID,
    created_by_type TEXT NOT NULL,
    created_by_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (id)
);
