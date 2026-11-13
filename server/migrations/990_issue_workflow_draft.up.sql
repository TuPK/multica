-- Drafts of a flow workflow (spec 2.2). The primary key is attached by 992 from the index
-- 991 builds concurrently; no foreign key and no CHECK: the application validates.
CREATE TABLE IF NOT EXISTS issue_workflow_draft (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    squad_id UUID NOT NULL,
    lineage_id UUID,
    base_workflow_id UUID,
    name TEXT NOT NULL,
    doc JSONB NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    origin TEXT NOT NULL DEFAULT 'manual',
    source_ref JSONB,
    created_by UUID NOT NULL,
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
