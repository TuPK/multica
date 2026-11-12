-- Per-workspace Ayrshare keys (Content Studio). Each key is sealed with the
-- publish secret key (MULTICA_PUBLISH_SECRET_KEY, secretbox) and is NULL when not
-- set. The API never returns key material; it reports only whether a key is set.
--
-- x_key_sealed and x_secret_sealed are the workspace's own X developer keys, which
-- Ayrshare requires for X posts. No foreign keys: the writer checks the workspace.
CREATE TABLE IF NOT EXISTS publish_settings (
    workspace_id         UUID PRIMARY KEY,
    ayrshare_key_sealed  BYTEA,
    x_key_sealed         BYTEA,
    x_secret_sealed      BYTEA,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
