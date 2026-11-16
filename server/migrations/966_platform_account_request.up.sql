-- platform_account_request: an admin's request to create an account for a
-- person (platform management spec 7b.5). The platform writes it; Multica's
-- account_provisioning job writes state, user_id, workspace_id, error_code,
-- attempts and updated_at. The unique indexes are migrations 967 and 968.
CREATE TABLE IF NOT EXISTS platform_account_request (
    id uuid NOT NULL,
    kind text NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL,
    workspace_name text NOT NULL,
    workspace_slug text NOT NULL DEFAULT '',
    plan_key text NOT NULL,
    seats int NOT NULL DEFAULT 1,
    interval text NOT NULL DEFAULT 'month',
    trial boolean NOT NULL DEFAULT false,
    comp_reason text NOT NULL DEFAULT '',
    billing_profile jsonb,
    state text NOT NULL DEFAULT 'pending',
    error_code text NOT NULL DEFAULT '',
    attempts int NOT NULL DEFAULT 0,
    user_id uuid,
    workspace_id uuid,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    welcome_sent_at timestamptz,
    welcome_attempts int NOT NULL DEFAULT 0
);
