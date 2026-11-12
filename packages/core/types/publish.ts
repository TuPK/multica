/**
 * A publishing channel of a workspace, as GET /api/publish-channels returns it.
 * Mirrors `PublishChannelResponse` in `server/internal/handler/publish_channel.go`.
 *
 * `platform`, `market` and `status` are strings on purpose: a newer server may
 * add a value, and an older client must still render the row. Parsing (see
 * publish-channels/schema.ts) maps an unknown status to "error".
 */
export type PublishChannelStatus = "not_connected" | "ready" | "error" | "expired";

export interface PublishChannelLimits {
  max_chars: number;
  max_video_seconds: number | null;
  aspect: string | null;
}

export interface PublishChannel {
  id: string;
  platform: string;
  market: string;
  account_name: string;
  status: PublishChannelStatus;
  status_detail: string;
  /** RFC 3339 time of the last check, or null when never checked. */
  last_checked_at: string | null;
  limits: PublishChannelLimits;
}

/** The body of POST /api/publish-channels. */
export interface CreatePublishChannelInput {
  platform: string;
  market: string;
  account_name: string;
}

/**
 * Which Ayrshare keys a workspace has set. The API never returns key material.
 * `x_configured` is true only when both X keys are set.
 */
export interface PublishSettings {
  ayrshare_configured: boolean;
  x_configured: boolean;
}

/**
 * The body of PUT /api/publish-channels/settings. A field left out is unchanged;
 * an empty string clears that key.
 */
export interface SavePublishSettingsInput {
  ayrshare_api_key?: string;
  x_api_key?: string;
  x_api_secret?: string;
}
