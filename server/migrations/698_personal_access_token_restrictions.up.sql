-- Restricted personal access tokens (platform management spec 6.5).
-- workspace_id binds a token to one workspace; NULL means any workspace, as for
-- every token that exists today. read_only refuses every unsafe method. Both
-- defaults leave existing tokens with full access. No foreign key (the
-- workspace is validated in application code) and no index (the token is
-- looked up by token_hash, which is already unique).
ALTER TABLE personal_access_token
    ADD COLUMN IF NOT EXISTS workspace_id uuid,
    ADD COLUMN IF NOT EXISTS read_only boolean NOT NULL DEFAULT false;
