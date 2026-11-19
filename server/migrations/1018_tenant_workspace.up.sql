-- Each workspace belongs to exactly one tenant (unique index of 1019). workspace
-- itself gets no column.
CREATE TABLE IF NOT EXISTS tenant_workspace (
    workspace_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
