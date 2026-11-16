-- The OIDC identity provider a workspace signs in through (platform spec 6.3).
-- client_secret_enc is the client secret sealed with the MFA key; the plaintext
-- is never stored. No primary key or foreign key: 697 makes the connection
-- unique per workspace.
CREATE TABLE sso_connection (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    issuer text NOT NULL,
    client_id text NOT NULL,
    client_secret_enc bytea NOT NULL,
    default_role text NOT NULL DEFAULT 'member',
    status text NOT NULL DEFAULT 'active',
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
