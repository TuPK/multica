-- When a squad finishes a request, it may hand the work to another squad (for
-- example planning to SDLC). One rule per source squad: the squad it hands to
-- and how. mode is off (the boss hands off by hand), ask (the handoff waits for
-- the boss's approval) or auto (it runs at once). No foreign keys: squad ids are
-- application-layer relations, and workspace deletion removes the rows itself.
CREATE TABLE squad_handoff_rule (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    source_squad_id UUID NOT NULL,
    target_squad_id UUID NOT NULL,
    mode TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'ask', 'auto')),
    updated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
