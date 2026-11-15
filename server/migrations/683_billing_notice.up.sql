CREATE TABLE IF NOT EXISTS billing_notice (
    workspace_id uuid NOT NULL,
    notice_key text NOT NULL,
    kind text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
