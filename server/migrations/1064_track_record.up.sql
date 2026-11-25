-- Track record (durable spec 3.2, A6): settled outcome counts per (agent, work class,
-- window). model '' is the merged row; per-model rows come with F8. unverified
-- outcomes are counted apart and never reach score_lower. mode_changed_at is when the
-- verification mode last changed, which the 7-day rule reads.
--
-- No foreign keys (project rule). The unique key is a separate index, built
-- concurrently in 1065.
CREATE TABLE track_record (
    workspace_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    work_class TEXT NOT NULL CHECK (char_length(work_class) <= 200),
    window_days INTEGER NOT NULL CHECK (window_days IN (30, 90)),
    model TEXT NOT NULL DEFAULT '',
    n_pass INTEGER NOT NULL DEFAULT 0,
    n_partial INTEGER NOT NULL DEFAULT 0,
    n_fail INTEGER NOT NULL DEFAULT 0,
    n_unverified INTEGER NOT NULL DEFAULT 0,
    n_high_fail INTEGER NOT NULL DEFAULT 0,
    score_lower DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_high_fail_at TIMESTAMPTZ,
    verification_mode TEXT NOT NULL DEFAULT 'full'
        CHECK (verification_mode IN ('full', 'sampled', 'spot')),
    mode_pinned BOOLEAN NOT NULL DEFAULT false,
    mode_changed_at TIMESTAMPTZ,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
