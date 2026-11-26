-- Shadow pairs (durable spec 9.4): one sampled primary task and the shadow task that
-- ran beside it, with the blind comparison once a juror has judged them. The comparison
-- is written from the primary's side: primary, shadow, tie or unable. No foreign keys.
CREATE TABLE IF NOT EXISTS shadow_pair (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    config_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    primary_task_id UUID NOT NULL,
    shadow_task_id UUID,
    comparison TEXT CHECK (comparison IN ('primary', 'shadow', 'tie', 'unable')),
    comparison_reason TEXT NOT NULL DEFAULT '' CHECK (char_length(comparison_reason) <= 2000),
    primary_cost_micros BIGINT NOT NULL DEFAULT 0,
    shadow_cost_micros BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    compared_at TIMESTAMPTZ
);
