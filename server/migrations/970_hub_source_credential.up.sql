-- The access token of a private source (GitHub, URL zip or registry), sealed with
-- the Hub secret key (MULTICA_HUB_SECRET_KEY). NULL means no credential. No
-- foreign keys: the source row owns this column.
ALTER TABLE hub_source ADD COLUMN IF NOT EXISTS credential_ciphertext BYTEA;
