-- Shadow run configs (durable spec 9.4, F8): one trial of a candidate runtime and model
-- on a seat. The candidate runs as a shadow agent, which shadow_agent_id names once the
-- first sampled task creates it. No foreign keys (project rule): the service checks the
-- workspace, the seat and the runtime, and nothing cascades.
CREATE TABLE IF NOT EXISTS shadow_config (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    candidate_runtime_id UUID NOT NULL,
    candidate_model TEXT NOT NULL DEFAULT '' CHECK (char_length(candidate_model) <= 200),
    purpose TEXT NOT NULL CHECK (purpose IN ('model_trial', 'necessity_test')),
    necessity_test_id UUID,
    work_class_prefix TEXT NOT NULL DEFAULT '' CHECK (char_length(work_class_prefix) <= 200),
    sample_rate NUMERIC NOT NULL DEFAULT 0.3 CHECK (sample_rate > 0 AND sample_rate <= 1),
    target_pairs INTEGER NOT NULL DEFAULT 20 CHECK (target_pairs BETWEEN 5 AND 200),
    auto_promote BOOLEAN NOT NULL DEFAULT false,
    shadow_agent_id UUID,
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'completed', 'stopped')),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
