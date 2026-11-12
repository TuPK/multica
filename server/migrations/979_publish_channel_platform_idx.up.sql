-- One channel per platform in a workspace: an Ayrshare profile holds one link per
-- network, so a second channel on the same platform could not be connected.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS publish_channel_workspace_platform_idx
    ON publish_channel (workspace_id, platform);
