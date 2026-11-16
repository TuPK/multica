-- billing_profile: a workspace's account kind and billing identity (platform
-- management spec 7b.1). A workspace with no row is an individual account, so
-- no backfill is needed. The unique index on (workspace_id) is migration 955.
CREATE TABLE IF NOT EXISTS billing_profile (
    workspace_id uuid NOT NULL,
    account_kind text NOT NULL DEFAULT 'individual',
    legal_name text NOT NULL DEFAULT '',
    tax_code text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    invoice_email text NOT NULL DEFAULT '',
    verified_at timestamptz,
    verified_by uuid,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now()
);
