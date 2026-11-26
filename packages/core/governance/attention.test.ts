import { describe, expect, it } from "vitest";
import { FALLBACK_ATTENTION_BUDGET, parseAttentionBudget } from "./attention";

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
