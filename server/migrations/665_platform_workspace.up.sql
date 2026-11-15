CREATE TABLE IF NOT EXISTS platform_workspace (
    workspace_id uuid NOT NULL,
    status text NOT NULL,
    reason text NOT NULL DEFAULT '',
    changed_by uuid,
    changed_at timestamptz NOT NULL DEFAULT now(),
    deletion_scheduled_at timestamptz
);
