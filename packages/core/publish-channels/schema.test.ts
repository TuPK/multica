// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  parsePublishChannel,
  parsePublishChannels,
  parsePublishConnect,
  parsePublishDryRunBody,
  parsePublishSettings,
} from "./schema";

const CHANNEL = {
  id: "c1",
  platform: "x",
  market: "intl",
  account_name: "@founder",
  status: "ready",
  status_detail: "",
  last_checked_at: "2026-11-12T08:00:00Z",
  limits: { max_chars: 280, max_video_seconds: null, aspect: null },
};

describe("parsePublishChannels", () => {
  it("reads the list the server returns", () => {
    const [channel] = parsePublishChannels({ channels: [CHANNEL] });
    expect(channel).toEqual(CHANNEL);
  });

  it("reads a malformed answer as no channels instead of throwing", () => {
    expect(parsePublishChannels("not an object")).toEqual([]);
    expect(parsePublishChannels({ channels: "nope" })).toEqual([]);
    expect(parsePublishChannels(null)).toEqual([]);
  });

  it("maps an unknown status to error so the row still renders", () => {
    const [channel] = parsePublishChannels({ channels: [{ ...CHANNEL, status: "paused_by_newer_server" }] });
    expect(channel?.status).toBe("error");
  });

  it("fills defaults for missing fields and keeps unknown platforms", () => {
    const [channel] = parsePublishChannels({ channels: [{ id: "c2", platform: "threads_v2" }] });
    expect(channel).toEqual({
      id: "c2",
      platform: "threads_v2",
      market: "",
      account_name: "",
      status: "error",
      status_detail: "",
      last_checked_at: null,
      limits: { max_chars: 0, max_video_seconds: null, aspect: null },
    });
  });

  it("drops a row that is not a channel without hiding the others", () => {
    const list = parsePublishChannels({ channels: [CHANNEL, "junk"] });
    expect(list).toEqual([CHANNEL]);
  });
});

describe("parsePublishChannel", () => {
  it("falls back to an empty error channel for a malformed answer", () => {
    const channel = parsePublishChannel(42);
    expect(channel.id).toBe("");
    expect(channel.status).toBe("error");
  });
});

describe("parsePublishConnect", () => {
  it("returns the link", () => {
    expect(parsePublishConnect({ url: "https://profile.ayrshare.com?session=s" })).toEqual({
      url: "https://profile.ayrshare.com?session=s",
    });
  });

  it("returns an empty url for a malformed answer", () => {
    expect(parsePublishConnect({ url: 5 })).toEqual({ url: "" });
  });
});

describe("parsePublishDryRunBody", () => {
  it("returns the request body", () => {
    expect(parsePublishDryRunBody({ body: { post: "hi", platforms: ["twitter"] } })).toEqual({
      post: "hi",
      platforms: ["twitter"],
    });
  });

  it("returns an empty body for a malformed answer", () => {
    expect(parsePublishDryRunBody({ body: "x" })).toEqual({});
  });
});

describe("parsePublishSettings", () => {
  it("reads which keys are set", () => {
    expect(parsePublishSettings({ ayrshare_configured: true, x_configured: false })).toEqual({
      ayrshare_configured: true,
      x_configured: false,
    });
  });

  it("defaults a missing flag to not configured", () => {
    expect(parsePublishSettings({ ayrshare_configured: true })).toEqual({
      ayrshare_configured: true,
      x_configured: false,
    });
  });

  it("reads a malformed answer as nothing configured instead of throwing", () => {
    expect(parsePublishSettings("not an object")).toEqual({ ayrshare_configured: false, x_configured: false });
    expect(parsePublishSettings({ ayrshare_configured: "yes" })).toEqual({
      ayrshare_configured: false,
      x_configured: false,
    });
  });
});
