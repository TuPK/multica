CREATE TABLE IF NOT EXISTS platform_account (
    user_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'pending_deletion', 'deleted')),
    reason text NOT NULL DEFAULT '',
    changed_by uuid,
    changed_at timestamptz NOT NULL DEFAULT now(),
    sessions_valid_after timestamptz,
    deletion_scheduled_at timestamptz
);
