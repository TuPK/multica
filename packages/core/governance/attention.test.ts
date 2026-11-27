// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_QUEUED_ATTENTION,
  FALLBACK_ATTENTION_BUDGET,
  parseAttentionBudget,
  parseQueuedAttention,
} from "./attention";

describe("parseAttentionBudget", () => {
  it("reads a budget as the server sends it", () => {
    expect(
      parseAttentionBudget({
        enabled: true,
        minutes_per_week: 90,
        split: "goals",
        week_start: "2026-11-23",
        spent_minutes: 23.5,
      }),
    ).toEqual({
      enabled: true,
      minutes_per_week: 90,
      split: "goals",
      week_start: "2026-11-23",
      spent_minutes: 23.5,
    });
  });

  it("falls back field by field on a malformed or newer response", () => {
    expect(
      parseAttentionBudget({ enabled: "yes", minutes_per_week: -4, split: "by_weather", spent_minutes: "lots" }),
    ).toEqual({
      enabled: false,
      minutes_per_week: 60,
      split: "equal",
      week_start: "",
      spent_minutes: 0,
    });
  });

  it("returns the fallback for a body that is not an object", () => {
    expect(parseAttentionBudget(null)).toEqual(FALLBACK_ATTENTION_BUDGET);
    expect(parseAttentionBudget("oops")).toEqual(FALLBACK_ATTENTION_BUDGET);
  });
});

describe("parseQueuedAttention", () => {
  it("reads queued asks as the server sends them", () => {
    const payload = {
      asks: [
        {
          subject_type: "proposal",
          subject_id: "00000000-0000-0000-0000-000000000001",
          reason: "judgment",
          estimated_minutes: 3.5,
          created_at: "2026-11-26T08:00:00Z",
        },
      ],
    };
    expect(parseQueuedAttention(payload)).toEqual(payload);
  });

  it("degrades unknown subject_type and reason to unknown, defaults numbers, and drops rows without subject_id", () => {
    expect(
      parseQueuedAttention({
        asks: [
          null,
          { subject_type: "proposal", subject_id: "" },
          {
            subject_type: "future_subject",
            subject_id: "ask-1",
            reason: "future_reason",
            estimated_minutes: -2,
          },
        ],
      }),
    ).toEqual({
      asks: [
        {
          subject_type: "unknown",
          subject_id: "ask-1",
          reason: "unknown",
          estimated_minutes: 0,
          created_at: "",
        },
      ],
    });
  });

  it("returns empty asks for a non-object body", () => {
    expect(parseQueuedAttention(null)).toEqual(EMPTY_QUEUED_ATTENTION);
    expect(parseQueuedAttention("bad")).toEqual(EMPTY_QUEUED_ATTENTION);
  });
});
