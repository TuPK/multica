-- Seat dossier entries (durable spec 9.2, F8): what an agent seat has learned, kept with
-- the seat, not with the model in it. A model change keeps these rows. No foreign keys
-- (project rule): the writer checks the workspace and the agent, and the monthly cleanup
-- retires entries; it never deletes them.
CREATE TABLE IF NOT EXISTS seat_dossier_entry (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('lesson', 'pitfall', 'preference', 'context', 'contact')),
    text TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 800),
    work_class TEXT NOT NULL DEFAULT '' CHECK (char_length(work_class) <= 200),
    evidence JSONB NOT NULL CHECK (jsonb_typeof(evidence) = 'array' AND jsonb_array_length(evidence) >= 1),
    origin TEXT NOT NULL CHECK (origin IN ('outcome', 'retro', 'agent', 'member')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    uses INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at TIMESTAMPTZ
);
