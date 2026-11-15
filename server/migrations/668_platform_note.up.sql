CREATE TABLE IF NOT EXISTS platform_note (
    workspace_id uuid NOT NULL,
    author_id uuid NOT NULL,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    id uuid NOT NULL DEFAULT gen_random_uuid()
);
