CREATE TABLE IF NOT EXISTS member_removal (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    removed_at timestamptz NOT NULL DEFAULT now()
);
