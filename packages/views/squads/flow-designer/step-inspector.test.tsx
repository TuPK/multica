// @vitest-environment jsdom

import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc, FlowDocStep } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { StepInspector } from "./step-inspector";

// plan -> collect -> draft -> gate -> done, with a way back from gate to draft along a loop, and a side step
// "audit" that nothing leads from to draft.
const DOC = {
  version: 2,
  start: "plan",
  roles: [{ key: "writer", kind: "agent" }],
  loops: [{ key: "fix", header: "draft", max: 3 }],
  terminals: [{ key: "done", status: "done" }],
  steps: [
    { key: "plan", title: "Plan", kind: "tasks", tasks: [{ key: "scope", title: "Scope", kind: "work", role: "writer" }], edges: [{ id: "p1", to: "collect" }] },
    {
      key: "collect",
      title: "Collect",
      kind: "tasks",
      tasks: [{ key: "gather", title: "Gather", kind: "work", role: "writer", requires: { files: [{ slot: "report", min: 1 }, { slot: "data", min: 1 }] } }],
      edges: [{ id: "c1", to: "draft" }],
    },
    { key: "draft", title: "Draft", kind: "tasks", tasks: [{ key: "write", title: "Write", kind: "work", role: "writer" }], edges: [{ id: "d1", to: "gate" }] },
    {
      key: "gate",
      title: "Gate",
      kind: "tasks",
      tasks: [{ key: "check", title: "Check", kind: "gate", role: "writer" }],
      edges: [{ id: "g1", to: "draft", loop: "fix" }, { id: "g2", to: "done" }],
    },
    { key: "audit", title: "Audit", kind: "tasks", tasks: [{ key: "audit_it", title: "Audit it", kind: "work", role: "writer" }], edges: [{ id: "a1", to: "done" }] },
  ],
} as unknown as FlowDoc;

const stepOf = (key: string) => DOC.steps.find((s) => s.key === key) as FlowDocStep;

afterEach(cleanup);

describe("StepInspector inputs", () => {
  it("offers only the tasks of other steps that lead forward to this step", () => {
    renderWithI18n(<StepInspector step={stepOf("draft")} onChangeStep={vi.fn()} doc={DOC} />);
    // plan and collect lead to draft. gate comes after it (its way back is a loop, not a forward path), audit is off the path.
    expect(screen.getByText("plan.scope")).toBeTruthy();
    expect(screen.getByText("collect.gather")).toBeTruthy();
    expect(screen.queryByText("gate.check")).toBeNull();
    expect(screen.queryByText("audit.audit_it")).toBeNull();
    // Not its own tasks either.
    expect(screen.queryByText("draft.write")).toBeNull();
  });

  it("gives the file slots of the source task to the slot picker", () => {
    const step = { ...stepOf("draft"), tasks: [{ ...stepOf("draft").tasks![0]!, inputs: [{ from: "collect.gather", required: false }] }] } as FlowDocStep;
    renderWithI18n(<StepInspector step={step} onChangeStep={vi.fn()} doc={DOC} />);
    const slot = screen.getByRole("combobox", { name: "File slot of collect.gather" }) as HTMLSelectElement;
    expect([...slot.options].map((o) => o.value)).toEqual(["", "report", "data"]);
  });
});

describe("StepInspector quorum", () => {
  const quorumStep = {
    key: "review",
    title: "Review",
    kind: "tasks",
    join: { over: "all", mode: "quorum", block: "immediate", k: 2 },
    tasks: ["a", "b", "c"].map((k) => ({ key: k, title: k, kind: "gate", role: "writer" })),
    edges: [],
  } as unknown as FlowDocStep;

  it("changes the number of approvals, between one and the number of tasks", () => {
    const onChangeStep = vi.fn();
    renderWithI18n(<StepInspector step={quorumStep} onChangeStep={onChangeStep} doc={DOC} />);
    const k = screen.getByRole("spinbutton", { name: "Quorum" }) as HTMLInputElement;
    expect(k.value).toBe("2");
    expect(k.min).toBe("1");
    expect(k.max).toBe("3");
    fireEvent.change(k, { target: { value: "3" } });
    expect(onChangeStep).toHaveBeenLastCalledWith({ ...quorumStep, join: { ...quorumStep.join, k: 3 } });
    // A number past the tasks, or below one, is kept inside the range.
    fireEvent.change(k, { target: { value: "9" } });
    expect(onChangeStep).toHaveBeenLastCalledWith({ ...quorumStep, join: { ...quorumStep.join, k: 3 } });
    fireEvent.change(k, { target: { value: "0" } });
    expect(onChangeStep).toHaveBeenLastCalledWith({ ...quorumStep, join: { ...quorumStep.join, k: 1 } });
  });

  it("has no number of approvals for the join modes that do not use one", () => {
    renderWithI18n(<StepInspector step={{ ...quorumStep, join: { over: "all", mode: "all", block: "immediate" } } as FlowDocStep} onChangeStep={vi.fn()} doc={DOC} />);
    expect(screen.queryByRole("spinbutton", { name: "Quorum" })).toBeNull();
  });
});

describe("StepInspector skills of a role", () => {
  it("offers a task the skills of its own role", () => {
    const skillsOfRole = (role: string) => (role === "writer" ? ["tone-check"] : []);
    renderWithI18n(<StepInspector step={stepOf("draft")} onChangeStep={vi.fn()} doc={DOC} skillsOfRole={skillsOfRole} />);
    expect(screen.getByText("tone-check")).toBeTruthy();
  });
});
