// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ATTENTION_OPENED_HEADER, attentionOpenedHeaders } from "./attention-opened";

afterEach(() => {
  vi.useRealTimers();
});

describe("attentionOpenedHeaders", () => {
  it("exports the canonical header name", () => {
    expect(ATTENTION_OPENED_HEADER).toBe("X-Attention-Opened-At");
  });

  it("returns whole elapsed seconds rounded down", () => {
    expect(attentionOpenedHeaders(10_000, 55_000)).toEqual({
      "X-Attention-Opened-At": "45",
    });
    expect(attentionOpenedHeaders(10_000, 55_999)).toEqual({
      "X-Attention-Opened-At": "45",
    });
    expect(attentionOpenedHeaders(10_000, 10_999)).toEqual({
      "X-Attention-Opened-At": "0",
    });
  });

  it("clamps negative durations to 0 when openedAt is in the future", () => {
    expect(attentionOpenedHeaders(20_000, 15_000)).toEqual({
      "X-Attention-Opened-At": "0",
    });
  });

  it("returns an empty object when openedAt is undefined or non-finite", () => {
    expect(attentionOpenedHeaders(undefined, 50_000)).toEqual({});
    expect(attentionOpenedHeaders(Number.NaN, 50_000)).toEqual({});
    expect(attentionOpenedHeaders(Number.POSITIVE_INFINITY, 50_000)).toEqual({});
    expect(attentionOpenedHeaders(Number.NEGATIVE_INFINITY, 50_000)).toEqual({});
  });

  it("defaults nowMs to Date.now()", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_030_500);
    expect(attentionOpenedHeaders(1_700_000_000_000)).toEqual({
      "X-Attention-Opened-At": "30",
    });
  });
});
