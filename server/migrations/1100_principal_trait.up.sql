-- A trait of a principal profile (durable spec 6.2): one statement about how a person decides,
-- with the evidence it rests on. Retired traits stay as history (spec 6.3). No foreign keys.
CREATE TABLE principal_trait (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    decision_kind TEXT NOT NULL,
    statement TEXT NOT NULL CHECK (char_length(statement) BETWEEN 1 AND 500),
    evidence JSONB NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('learned', 'interview', 'edited')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at TIMESTAMPTZ
);
