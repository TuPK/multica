-- A tenant holds one or more workspaces (spec R6 section 5). No foreign keys and
-- no constraints other than the unique index of 1017: the application keeps the
-- relation to workspaces (tenant_workspace).
CREATE TABLE IF NOT EXISTS tenant (
    id uuid NOT NULL,
    name text NOT NULL,
    created_by uuid,
    status text NOT NULL DEFAULT 'active',
    status_reason text NOT NULL DEFAULT '',
    status_changed_at timestamptz,
    status_changed_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
