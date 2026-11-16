// @vitest-environment jsdom

import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc, FlowIssue } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { ProblemsPanel } from "./problems-panel";

const DOC = {
  version: 2,
  start: "plan",
  terminals: [{ key: "done", status: "done" }],
  steps: [
    { key: "plan", title: "Plan", kind: "lead", edges: [] },
    { key: "collect", title: "Collect", kind: "tasks", tasks: [], edges: [] },
    { key: "gate", title: "Gate", kind: "tasks", tasks: [], edges: [] },
  ],
} as unknown as FlowDoc;

const issue = (over: Partial<FlowIssue>): FlowIssue => ({ code: "x", severity: "error", message: "m", ...over });

afterEach(cleanup);

describe("ProblemsPanel", () => {
  const issues: FlowIssue[] = [
    issue({ code: "step_unreachable", path: "steps[2]", message: "Gate cannot be reached" }),
    issue({ code: "task_role_missing", path: "steps[1].tasks[0].role", message: "No such role" }),
    issue({ code: "loop_unbounded", severity: "warning", path: "loops[0]", params: { step: "collect" }, message: "Loop has no end" }),
    issue({ code: "no_start", path: "start", message: "No start step" }),
  ];

  it("groups the issues under the step they belong to, in the order of the steps, and the rest first", () => {
    renderWithI18n(<ProblemsPanel issues={issues} doc={DOC} />);
    const groups = screen.getAllByRole("group");
    expect(groups.map((g) => within(g).getByRole("heading").textContent)).toEqual(["Collect", "Gate"]);
    expect(within(groups[0]!).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      expect.stringContaining("task_role_missing"),
      expect.stringContaining("loop_unbounded"),
    ]);
    expect(within(groups[1]!).getAllByRole("listitem")).toHaveLength(1);
    // The issue that belongs to no step is on screen, before the groups.
    const all = screen.getAllByRole("listitem").map((li) => li.textContent ?? "");
    expect(all[0]).toContain("no_start");
    expect(all).toHaveLength(4);
  });

  it("selects the step an issue is about when it is clicked, and none for an issue of no step", () => {
    const onSelectIssue = vi.fn();
    renderWithI18n(<ProblemsPanel issues={issues} doc={DOC} onSelectIssue={onSelectIssue} />);
    fireEvent.click(screen.getByText("Gate cannot be reached"));
    expect(onSelectIssue).toHaveBeenLastCalledWith(issues[0], "gate");
    fireEvent.click(screen.getByText("No such role"));
    expect(onSelectIssue).toHaveBeenLastCalledWith(issues[1], "collect");
    // A step named by params wins over a path that is not a step.
    fireEvent.click(screen.getByText("Loop has no end"));
    expect(onSelectIssue).toHaveBeenLastCalledWith(issues[2], "collect");
    fireEvent.click(screen.getByText("No start step"));
    expect(onSelectIssue).toHaveBeenLastCalledWith(issues[3], null);
  });

  it("marks a warning that needs an acknowledgement, and counts errors and warnings", () => {
    renderWithI18n(<ProblemsPanel issues={[issue({ severity: "warning", code: "release_without_human", ack: "release_without_human" }), issue({})]} doc={DOC} />);
    expect(screen.getByText("Needs acknowledgement")).toBeInTheDocument();
    expect(screen.getByText("1 error")).toBeInTheDocument();
    expect(screen.getByText("1 warning")).toBeInTheDocument();
  });

  it("says so when there is nothing to fix", () => {
    renderWithI18n(<ProblemsPanel issues={[]} doc={DOC} />);
    expect(screen.getByText("No problems found")).toBeInTheDocument();
  });
});
