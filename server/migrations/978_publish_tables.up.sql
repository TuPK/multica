-- Publishing channels and attempts (Content Studio phase 1-3).
--
-- publish_channel: one connected publishing account of a workspace. The account
-- is linked through Ayrshare; the link itself lives on the Ayrshare profile.
--
-- publish_profile: the Ayrshare User Profile of a workspace, one row per
-- workspace. profile_key_sealed holds the Profile-Key sealed with the publish
-- secret key (MULTICA_PUBLISH_SECRET_KEY, secretbox), the same at-rest pattern
-- as the Lark and Hub secrets. The key is never returned to a client.
--
-- publish_attempt: one send of one channel version of a publish proposal. Its
-- unique key (workspace_id, item_id, channel_id, version) is the idempotency key
-- and is created in its own migration (980), so one approval cannot enqueue the
-- same send twice.
--
-- No foreign keys: the writers check the workspace and the channel in code.
CREATE TABLE IF NOT EXISTS publish_channel (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     UUID NOT NULL,
    platform         TEXT NOT NULL CHECK (platform IN ('x', 'linkedin', 'facebook', 'tiktok', 'youtube', 'threads', 'bluesky')),
    market           TEXT NOT NULL CHECK (market IN ('vn', 'intl')),
    account_name     TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'not_connected' CHECK (status IN ('not_connected', 'ready', 'error', 'expired')),
    status_detail    TEXT NOT NULL DEFAULT '',
    last_checked_at  TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS publish_profile (
    workspace_id       UUID PRIMARY KEY,
    profile_key_sealed BYTEA NOT NULL,
    ref_id             TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS publish_attempt (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       UUID NOT NULL,
    proposal_id        UUID NOT NULL,
    item_id            TEXT NOT NULL,
    channel_id         UUID NOT NULL,
    version            INTEGER NOT NULL CHECK (version >= 1),
    status             TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'posted', 'failed')),
    external_post_id   TEXT NOT NULL DEFAULT '',
    external_url       TEXT NOT NULL DEFAULT '',
    error              TEXT NOT NULL DEFAULT '',
    approved_by        TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
