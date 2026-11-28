// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_REACH_MAP,
  parseCloseReachGapResponse,
  parseReachMap,
  reachSurfaceKind,
  reachSurfaceLabel,
} from "./schemas";

describe("parseReachMap", () => {
  it("parses a valid snapshot and preserves all known fields", () => {
    const map = parseReachMap({
      snapshot_date: "2026-11-27",
      agent_share_30d: 62.5,
      surfaces: [
        {
          surface: "app:gmail",
          state: "agent_operable",
          effects_30d: 40,
          agent_effects_30d: 35,
          human_minutes_30d: 18.5,
          gap_asks_30d: 0,
          top_ask_ids: ["ask-1"],
          gap_issue_id: "issue-1",
        },
      ],
    });

    expect(map).toEqual({
      snapshot_date: "2026-11-27",
      agent_share_30d: 62.5,
      surfaces: [
        {
          surface: "app:gmail",
          state: "agent_operable",
          effects_30d: 40,
          agent_effects_30d: 35,
          human_minutes_30d: 18.5,
          gap_asks_30d: 0,
          top_ask_ids: ["ask-1"],
          gap_issue_id: "issue-1",
        },
      ],
    });
  });

  it("degrades an unknown surface state to unknown", () => {
    const map = parseReachMap({
      snapshot_date: "2026-11-27",
      agent_share_30d: 50,
      surfaces: [
        {
          surface: "external:billing.example.com",
          state: "future_state_v2",
          effects_30d: 4,
          agent_effects_30d: 2,
          human_minutes_30d: 12,
          gap_asks_30d: 1,
          top_ask_ids: [],
          gap_issue_id: null,
        },
      ],
    });

    expect(map.surfaces).toHaveLength(1);
    expect(map.surfaces[0]?.state).toBe("unknown");
  });

  it("drops surface rows missing a surface name while keeping valid rows", () => {
    const map = parseReachMap({
      snapshot_date: "2026-11-27",
      agent_share_30d: 20,
      surfaces: [
        { state: "human_only", human_minutes_30d: 30 },
        { surface: "", state: "human_only", human_minutes_30d: 15 },
        { surface: "channel:slack", state: "agent_with_confirm", human_minutes_30d: 25 },
      ],
    });

    expect(map.surfaces).toHaveLength(1);
    expect(map.surfaces[0]?.surface).toBe("channel:slack");
  });

  it("coerces negative numbers and strings to 0", () => {
    const map = parseReachMap({
      snapshot_date: "2026-11-27",
      agent_share_30d: -10,
      surfaces: [
        {
          surface: "area:ops:billing",
          state: "declared_only",
          effects_30d: -5,
          agent_effects_30d: "12",
          human_minutes_30d: "not-a-number",
          gap_asks_30d: -3,
          top_ask_ids: ["", 123, "ask-2"],
          gap_issue_id: 42,
        },
      ],
    });

    expect(map.agent_share_30d).toBe(0);
    expect(map.surfaces[0]).toMatchObject({
      effects_30d: 0,
      agent_effects_30d: 0,
      human_minutes_30d: 0,
      gap_asks_30d: 0,
      top_ask_ids: ["ask-2"],
      gap_issue_id: null,
    });
  });

  it("returns EMPTY_REACH_MAP on malformed top-level payloads", () => {
    expect(parseReachMap(null)).toEqual(EMPTY_REACH_MAP);
    expect(parseReachMap("oops")).toEqual(EMPTY_REACH_MAP);
  });
});

describe("parseCloseReachGapResponse", () => {
  it("extracts issue_id on a valid payload", () => {
    expect(parseCloseReachGapResponse({ issue_id: "iss-1" })).toEqual({ issue_id: "iss-1" });
  });

  it("returns null when issue_id is missing or empty", () => {
    expect(parseCloseReachGapResponse({})).toBeNull();
    expect(parseCloseReachGapResponse({ issue_id: "" })).toBeNull();
    expect(parseCloseReachGapResponse(null)).toBeNull();
  });
});

describe("reachSurfaceKind and reachSurfaceLabel", () => {
  it("recognizes all four spec prefixes and falls back to other", () => {
    expect(reachSurfaceKind("app:gmail")).toBe("app");
    expect(reachSurfaceKind("channel:slack")).toBe("channel");
    expect(reachSurfaceKind("area:flow:pr")).toBe("area");
    expect(reachSurfaceKind("external:stripe.com")).toBe("external");
    expect(reachSurfaceKind("custom:thing")).toBe("other");
    expect(reachSurfaceKind("plain")).toBe("other");
  });

  it("strips known prefixes for display and preserves unknown or empty suffixes", () => {
    expect(reachSurfaceLabel("app:gmail")).toBe("gmail");
    expect(reachSurfaceLabel("channel:slack")).toBe("slack");
    expect(reachSurfaceLabel("area:flow:pr")).toBe("flow:pr");
    expect(reachSurfaceLabel("external:stripe.com")).toBe("stripe.com");
    expect(reachSurfaceLabel("custom:thing")).toBe("custom:thing");
    expect(reachSurfaceLabel("app:")).toBe("app:");
  });
});
