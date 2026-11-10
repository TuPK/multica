-- One row per request a squad hands to another. status is pending (waiting for
-- the boss, or for the company to run), handed_off (target_issue_id is the
-- request the other squad received) or declined. No foreign keys.
CREATE TABLE squad_handoff (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    source_squad_id UUID NOT NULL,
    target_squad_id UUID NOT NULL,
    source_issue_id UUID NOT NULL,
    target_issue_id UUID,
    mode TEXT NOT NULL CHECK (mode IN ('off', 'ask', 'auto')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'handed_off', 'declined')),
    requested_by_type TEXT NOT NULL CHECK (requested_by_type IN ('member', 'agent')),
    requested_by_id UUID NOT NULL,
    decided_by UUID,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ
);
