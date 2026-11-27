-- The Reach Map snapshot (durable spec 11.2, F10): one row per workspace, day and surface,
-- recomputed nightly from effect_event and attention_ledger. The unique index that makes a
-- second run of the same day overwrite its rows is built in 1121, concurrently. No foreign
-- keys (project rule): gap_issue_id is checked in application code.
CREATE TABLE reach_snapshot (
    workspace_id UUID NOT NULL,
    snapshot_date DATE NOT NULL,
    surface TEXT NOT NULL CHECK (char_length(surface) <= 300),
    state TEXT NOT NULL CHECK (state IN ('agent_operable', 'agent_with_confirm', 'declared_only', 'human_only', 'unknown')),
    effects_30d INTEGER NOT NULL DEFAULT 0,
    agent_effects_30d INTEGER NOT NULL DEFAULT 0,
    human_minutes_30d NUMERIC NOT NULL DEFAULT 0,
    gap_asks_30d INTEGER NOT NULL DEFAULT 0,
    top_ask_ids UUID[] NOT NULL DEFAULT '{}',
    gap_issue_id UUID,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
