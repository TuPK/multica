CREATE TABLE IF NOT EXISTS project_member (
    project_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    added_by uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);
