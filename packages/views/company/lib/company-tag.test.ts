// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Issue, Squad } from "@multica/core/types";
import { focusIssues } from "./company-tag";

function issue(id: string, status: string, assignee: { type: "agent" | "squad"; id: string } | null): Issue {
  return { id, status, assignee_type: assignee?.type ?? null, assignee_id: assignee?.id ?? null } as Issue;
}

const SQUADS = [{ id: "sq", leader_id: "lead" }] as Squad[];
const ALL = ["todo", "in_progress", "in_review", "done"];

describe("focusIssues", () => {
  it("prefers the issue in progress, then the first open one", () => {
    const map = focusIssues(
      [issue("a", "todo", { type: "agent", id: "x" }), issue("b", "in_progress", { type: "agent", id: "x" })],
      ALL,
      SQUADS,
    );
    expect(map.get("x")?.id).toBe("b");
  });

  it("counts a squad's issue for its leader", () => {
    const map = focusIssues([issue("a", "in_review", { type: "squad", id: "sq" })], ALL, SQUADS);
    expect(map.get("lead")?.id).toBe("a");
  });

  it("skips done issues, hidden statuses, unassigned issues and unknown squads", () => {
    const map = focusIssues(
      [
        issue("done", "done", { type: "agent", id: "x" }),
        issue("hidden", "blocked", { type: "agent", id: "x" }),
        issue("none", "todo", null),
        issue("ghost", "todo", { type: "squad", id: "nope" }),
      ],
      ALL,
      SQUADS,
    );
    expect(map.size).toBe(0);
  });
});
