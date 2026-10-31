// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  buildWalkPath,
  deskAnchor,
  pathLength,
  walkDurationMs,
  walkKeyframes,
} from "./walk-path";

describe("buildWalkPath", () => {
  it("leaves the room, crosses the corridor lane and enters the next room", () => {
    expect(buildWalkPath({ x: 40, y: 100 }, { x: 300, y: 80 }, 500)).toEqual([
      { x: 40, y: 100 },
      { x: 40, y: 500 },
      { x: 300, y: 500 },
      { x: 300, y: 80 },
    ]);
  });

  it("drops waypoints that repeat the previous one", () => {
    // Same column: the sideways leg has no length, so the walker never pauses.
    expect(buildWalkPath({ x: 40, y: 100 }, { x: 40, y: 220 }, 500)).toEqual([
      { x: 40, y: 100 },
      { x: 40, y: 500 },
      { x: 40, y: 220 },
    ]);
  });
});

describe("walkDurationMs", () => {
  it("keeps short hops readable and long walks bounded", () => {
    expect(walkDurationMs([{ x: 0, y: 0 }, { x: 0, y: 10 }])).toBe(700);
    expect(walkDurationMs([{ x: 0, y: 0 }, { x: 0, y: 100000 }])).toBe(2600);
  });
});

describe("walkKeyframes", () => {
  it("spaces offsets by distance and ends at 1", () => {
    const frames = walkKeyframes([
      { x: 0, y: 0 },
      { x: 0, y: 100 },
      { x: 300, y: 100 },
    ]);
    expect(pathLength([{ x: 0, y: 0 }, { x: 0, y: 100 }, { x: 300, y: 100 }])).toBe(400);
    expect(frames.map((frame) => frame.offset)).toEqual([0, 0.25, 1]);
  });

  it("faces the way it walks sideways", () => {
    const left = walkKeyframes([
      { x: 300, y: 0 },
      { x: 300, y: 100 },
      { x: 0, y: 100 },
    ]);
    expect(left.map((frame) => frame.facing)).toEqual([-1, -1, -1]);
    const right = walkKeyframes([
      { x: 0, y: 0 },
      { x: 300, y: 0 },
    ]);
    expect(right.map((frame) => frame.facing)).toEqual([1, 1]);
  });
});

describe("deskAnchor", () => {
  it("puts the feet below the worker's chair", () => {
    expect(deskAnchor({ x: 10, y: 20, w: 216, h: 92 })).toEqual({ x: 40, y: 86 });
  });
});
