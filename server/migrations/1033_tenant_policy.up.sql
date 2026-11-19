-- tenant_policy: the policy floor of a tenant (spec R6 section 3c.4, D101). The
-- same five fields as a workspace policy; a workspace may only tighten them. The
-- platform writes this table and Multica reads it. The unique index is migration 1034.
CREATE TABLE IF NOT EXISTS tenant_policy (
    tenant_id uuid NOT NULL,
    allowed_invite_domains text[] NOT NULL DEFAULT '{}',
    share_links_enabled boolean NOT NULL DEFAULT true,
    require_sso boolean NOT NULL DEFAULT false,
    require_mfa boolean NOT NULL DEFAULT false,
    session_max_age_hours integer NOT NULL DEFAULT 0,
    version bigint NOT NULL DEFAULT 1,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now()
);
