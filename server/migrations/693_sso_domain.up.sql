-- Domains a workspace claims for single sign-on (platform spec 6.3). A claim is
-- pending until the DNS TXT record that holds token is found, then verified_at
-- is set. No primary key or foreign key: 694 makes a domain verifiable by one
-- workspace only and 695 makes a claim unique per workspace.
CREATE TABLE sso_domain (
    workspace_id uuid NOT NULL,
    domain text NOT NULL,
    token text NOT NULL,
    verified_at timestamptz
);
