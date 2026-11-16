CREATE TABLE IF NOT EXISTS workspace_export (
    id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    status text NOT NULL,
    storage_key text NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    expires_at timestamptz
);
