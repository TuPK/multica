CREATE TABLE IF NOT EXISTS audit_event (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid,
    actor_type text NOT NULL,
    actor_id text,
    actor_label text NOT NULL DEFAULT '',
    action text NOT NULL,
    target_type text NOT NULL DEFAULT '',
    target_id text NOT NULL DEFAULT '',
    target_label text NOT NULL DEFAULT '',
    detail jsonb NOT NULL DEFAULT '{}',
    ip text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    request_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
