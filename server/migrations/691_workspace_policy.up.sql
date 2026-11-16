-- Workspace policies of the platform spec 6.4, one row per workspace. No row
-- means the defaults: any invitation domain, share links on, no second factor,
-- no SSO requirement, no session age limit. No primary key or foreign key: 692
-- adds the unique index on workspace_id.
CREATE TABLE workspace_policy (
    workspace_id uuid NOT NULL,
    allowed_invite_domains text[] NOT NULL DEFAULT '{}',
    share_links_enabled boolean NOT NULL DEFAULT true,
    require_sso boolean NOT NULL DEFAULT false,
    require_mfa boolean NOT NULL DEFAULT false,
    session_max_age_hours int NOT NULL DEFAULT 0,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now()
);
