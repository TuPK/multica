-- Acceptance criterion of an issue (durable spec 2.3, 2.5). Criteria are read by
-- the outcome settlement once A3 writes checks. No foreign keys (project rule).
CREATE TABLE acceptance_criterion (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    text TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 500),
    check_kind TEXT NOT NULL CHECK (check_kind IN
        ('manual', 'agent_judgment', 'command', 'artifact', 'metric')),
    check_spec JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL CHECK (source IN ('member', 'agent', 'flow', 'template')),
    created_by_type TEXT NOT NULL,
    created_by_id UUID,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
