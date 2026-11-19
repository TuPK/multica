-- platform_runtime: a runtime that staff disabled (spec R6 section 3b.5, D98). A
-- row means disabled; enabling deletes it. The platform writes this table and
-- Multica only reads it (its daemon guard). The unique index is migration 1032.
CREATE TABLE IF NOT EXISTS platform_runtime (
    runtime_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'disabled' CHECK (status = 'disabled'),
    reason text NOT NULL DEFAULT '',
    changed_by uuid NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now()
);
