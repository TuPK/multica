-- The principal profile (durable spec 6.2, F5): one row per member who consented to be learned
-- from. No foreign keys (project rule); the profile is removed by application code (spec 6.6).
CREATE TABLE principal_profile (
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'paused')),
    consent_at TIMESTAMPTZ NOT NULL,
    share_with_owners BOOLEAN NOT NULL DEFAULT false,
    version INTEGER NOT NULL DEFAULT 1,
    summary TEXT NOT NULL DEFAULT '' CHECK (char_length(summary) <= 4000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
