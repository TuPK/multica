-- A seat offered on the Hub (durable spec 13.3, F12, E4): the role, the squad it joins, the
-- runtime it asks for and the outcome it is hired on. Scoped to one Hub org, which is the
-- workspace that posts it. No foreign keys (project rule): the squad, runtime and creator are
-- checked in application code.
CREATE TABLE seat_offer (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    role TEXT NOT NULL DEFAULT '' CHECK (char_length(role) <= 300),
    squad_id UUID NOT NULL,
    runtime_id UUID NOT NULL,
    outcome_criteria TEXT NOT NULL DEFAULT '' CHECK (char_length(outcome_criteria) <= 2000),
    rationale TEXT NOT NULL CHECK (char_length(rationale) BETWEEN 1 AND 2000),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
