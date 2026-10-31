// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Issue } from "@multica/core/types";
import { planRoomMove } from "./plan-move";

function issue(id: string, position: number, status = "todo"): Issue {
  return { id, position, status } as Issue;
}

function mapOf(...issues: Issue[]) {
  return new Map(issues.map((i) => [i.id, i]));
}

describe("planRoomMove", () => {
  it("appends to the end of a populated room under manual order", () => {
    const moving = issue("m", 5);
    const a = issue("a", 1, "in_progress");
    const b = issue("b", 3, "in_progress");
    const plan = planRoomMove({
      issue: moving,
      status: "in_progress",
      targetIds: ["a", "b"],
      manualOrder: true,
      issueMap: mapOf(moving, a, b),
    });
    expect(plan).toEqual({ status: "in_progress", position: 4, before_id: "b", after_id: null });
  });

  it("keeps its own position when the room is empty", () => {
    const moving = issue("m", 5);
    const plan = planRoomMove({
      issue: moving,
      status: "done",
      targetIds: [],
      manualOrder: true,
      issueMap: mapOf(moving),
    });
    expect(plan).toEqual({ status: "done", position: 5, before_id: null, after_id: null });
  });

  it("keeps its position and lands by position under a sorted view", () => {
    const moving = issue("m", 2);
    const a = issue("a", 1, "in_progress");
    const b = issue("b", 3, "in_progress");
    const plan = planRoomMove({
      issue: moving,
      status: "in_progress",
      targetIds: ["a", "b"],
      manualOrder: false,
      issueMap: mapOf(moving, a, b),
    });
    expect(plan).toEqual({ status: "in_progress", position: 2, before_id: "a", after_id: "b" });
  });
});
