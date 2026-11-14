-- What a pack of skills and agents installed (spec 16.3). The primary key is attached by 996 from the
-- index 995 builds concurrently; no foreign key and no CHECK: the application validates. squad_id is empty
-- when the pack had no agents and only its skills were imported.
CREATE TABLE IF NOT EXISTS squad_pack_install (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    squad_id UUID,
    pack_id TEXT NOT NULL DEFAULT '',
    pack_version TEXT NOT NULL DEFAULT '',
    format TEXT NOT NULL,
    digest TEXT NOT NULL,
    source JSONB NOT NULL DEFAULT '{}'::jsonb,
    roster JSONB NOT NULL DEFAULT '{}'::jsonb,
    installed_by UUID NOT NULL,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
