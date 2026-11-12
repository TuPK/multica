// @vitest-environment node
import { describe, expect, it } from "vitest";
import { publishPayload, type Proposal } from "./proposal-schema";

const VALID = {
  item_id: "launch-post",
  version: 2,
  market: "intl",
  scheduled_at: null,
  channels: [
    {
      channel_id: "c1",
      platform: "x",
      account_name: "@founder",
      text: "We shipped it.",
      media_url: null,
      hashtags: ["launch"],
    },
  ],
};

function proposal(kind: string, payload: unknown): Pick<Proposal, "kind" | "payload"> {
  return { kind: kind as Proposal["kind"], payload: payload as Record<string, unknown> };
}

describe("publishPayload", () => {
  it("reads a valid publish payload", () => {
    expect(publishPayload(proposal("publish", VALID))).toEqual(VALID);
  });

  it("fills defaults for the optional channel fields", () => {
    const parsed = publishPayload(
      proposal("publish", {
        item_id: "a",
        version: 1,
        market: "vn",
        channels: [{ channel_id: "c1", text: "hello" }],
      }),
    );
    expect(parsed?.scheduled_at).toBeNull();
    expect(parsed?.channels[0]).toEqual({
      channel_id: "c1",
      platform: "",
      account_name: "",
      text: "hello",
      media_url: null,
      hashtags: [],
    });
  });

  it("reads a malformed payload as null and does not throw", () => {
    const malformed: unknown[] = [
      null,
      "not an object",
      { ...VALID, item_id: "" },
      { ...VALID, version: 0 },
      { ...VALID, version: 1.5 },
      { ...VALID, market: "mars" },
      { ...VALID, channels: [] },
      { ...VALID, channels: [{ text: "no channel id" }] },
    ];
    for (const payload of malformed) {
      expect(() => publishPayload(proposal("publish", payload))).not.toThrow();
      expect(publishPayload(proposal("publish", payload))).toBeNull();
    }
  });

  it("reads null for a proposal that is not a publish kind", () => {
    expect(publishPayload(proposal("decision", VALID))).toBeNull();
    expect(publishPayload(proposal("unknown", VALID))).toBeNull();
  });
});
