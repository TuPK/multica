CREATE TABLE IF NOT EXISTS workspace_subscription (
    workspace_id uuid NOT NULL,
    plan_key text NOT NULL,
    status text NOT NULL,
    seats int NOT NULL DEFAULT 0,
    interval text NOT NULL DEFAULT 'month',
    current_period_start timestamptz,
    current_period_end timestamptz,
    trial_end timestamptz,
    grace_until timestamptz,
    cancel_at_period_end boolean NOT NULL DEFAULT false,
    limit_overrides jsonb NOT NULL DEFAULT '{}',
    version bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);
