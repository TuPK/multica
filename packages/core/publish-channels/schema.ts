import { z } from "zod";
import { parseWithFallback } from "../api/schema";
import type { PublishChannel, PublishChannelStatus, PublishSettings } from "../types/publish";

/**
 * Zod schemas for the publishing-channel responses. Every field has a default,
 * and an unknown status maps to "error", so a newer server never blanks the tab.
 */

const STATUSES: readonly PublishChannelStatus[] = ["not_connected", "ready", "error", "expired"];

function toStatus(s: string): PublishChannelStatus {
  return (STATUSES as readonly string[]).includes(s) ? (s as PublishChannelStatus) : "error";
}

const NO_LIMITS = { max_chars: 0, max_video_seconds: null, aspect: null };

const PublishChannelLimitsSchema = z
  .object({
    max_chars: z.number().default(0),
    max_video_seconds: z.number().nullable().default(null),
    aspect: z.string().nullable().default(null),
  })
  .loose();

export const PublishChannelSchema = z
  .object({
    id: z.string().default(""),
    platform: z.string().default(""),
    market: z.string().default(""),
    account_name: z.string().default(""),
    status: z.string().default("error").transform(toStatus),
    status_detail: z.string().default(""),
    last_checked_at: z.string().nullable().default(null),
    limits: PublishChannelLimitsSchema.nullish().transform((l) => l ?? NO_LIMITS),
  })
  .loose();

// A row that does not parse is dropped on its own, so one bad row never hides
// the other channels.
export const PublishChannelListSchema = z
  .object({
    channels: z
      .array(z.unknown())
      .default([])
      .transform((rows) =>
        rows.flatMap((row) => {
          const parsed = PublishChannelSchema.safeParse(row);
          return parsed.success ? [parsed.data] : [];
        }),
      ),
  })
  .loose();

export const PublishChannelDryRunSchema = z
  .object({
    body: z.record(z.string(), z.unknown()).default({}),
  })
  .loose();

export const PublishChannelConnectSchema = z
  .object({
    url: z.string().default(""),
  })
  .loose();

const EMPTY_CHANNEL: PublishChannel = {
  id: "",
  platform: "",
  market: "",
  account_name: "",
  status: "error",
  status_detail: "",
  last_checked_at: null,
  limits: NO_LIMITS,
};

const EMPTY_CHANNELS: PublishChannel[] = [];

/** The channels of GET /api/publish-channels. A malformed answer reads as no channels. */
export function parsePublishChannels(raw: unknown): PublishChannel[] {
  return parseWithFallback(raw, PublishChannelListSchema, { channels: EMPTY_CHANNELS }, {
    endpoint: "GET /api/publish-channels",
  }).channels;
}

/** One channel from create, connect-check, or check. */
export function parsePublishChannel(raw: unknown): PublishChannel {
  return parseWithFallback(raw, PublishChannelSchema, EMPTY_CHANNEL, {
    endpoint: "publish-channels channel",
  });
}

/** The link from POST /connect. An empty url means the server gave no link. */
export function parsePublishConnect(raw: unknown): { url: string } {
  return parseWithFallback(raw, PublishChannelConnectSchema, { url: "" }, {
    endpoint: "POST /api/publish-channels/{id}/connect",
  });
}

/** The exact request body from POST /dry-run. */
export function parsePublishDryRunBody(raw: unknown): Record<string, unknown> {
  const empty: Record<string, unknown> = {};
  return parseWithFallback(raw, PublishChannelDryRunSchema, { body: empty }, {
    endpoint: "POST /api/publish-channels/{id}/dry-run",
  }).body;
}

/**
 * The settings answer: which keys are set. It never carries key material, so a
 * malformed answer reads as nothing configured.
 */
export const PublishSettingsSchema = z
  .object({
    ayrshare_configured: z.boolean().default(false),
    x_configured: z.boolean().default(false),
  })
  .loose();

const NOT_CONFIGURED: PublishSettings = { ayrshare_configured: false, x_configured: false };

/** Settings from GET or PUT /api/publish-channels/settings. */
export function parsePublishSettings(raw: unknown): PublishSettings {
  return parseWithFallback(raw, PublishSettingsSchema, NOT_CONFIGURED, {
    endpoint: "publish-channels settings",
  });
}
