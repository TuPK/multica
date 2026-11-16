// @vitest-environment jsdom

import type React from "react";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc, FlowDocTask } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { TaskCard } from "./task-card";

const DOC = { version: 2, start: "a", roles: [{ key: "writer", kind: "agent" }], steps: [], terminals: [], policy: {} } as unknown as FlowDoc;
const SKILLS = ["outline-first", "tone-check"];

function renderCard(task: FlowDocTask, skills: string[] = SKILLS, doc: FlowDoc = DOC, extra: Partial<React.ComponentProps<typeof TaskCard>> = {}) {
  const onChangeTask = vi.fn();
  renderWithI18n(<TaskCard task={task} onChangeTask={onChangeTask} onDeleteTask={vi.fn()} doc={doc} currentStepKey="write" roleSkills={skills} {...extra} />);
  return onChangeTask;
}

const TASK = { key: "write", title: "Write", kind: "work", role: "writer" } as FlowDocTask;

describe("TaskCard skills", () => {
  afterEach(cleanup);

  it("offers the skills of the squad and puts the one that is clicked in Task.skills", () => {
    const onChangeTask = renderCard(TASK);
    fireEvent.click(screen.getByText("outline-first"));
    expect(onChangeTask).toHaveBeenCalledWith({ ...TASK, skills: ["outline-first"] });
  });

  it("adds to the skills the task has, and takes one away when it is clicked again", () => {
    const onChangeTask = renderCard({ ...TASK, skills: ["outline-first"] } as FlowDocTask);
    fireEvent.click(screen.getByText("tone-check"));
    expect(onChangeTask).toHaveBeenLastCalledWith({ ...TASK, skills: ["outline-first", "tone-check"] });
    fireEvent.click(screen.getByText("outline-first"));
    expect(onChangeTask).toHaveBeenLastCalledWith({ ...TASK, skills: [] });
  });

  it("says which skill of the task the squad does not have", () => {
    renderCard({ ...TASK, skills: ["data-check"] } as FlowDocTask);
    expect(screen.getByText("R09: the role lacks skills: data-check")).toBeTruthy();
  });

  it("says the role has no skills when it has none", () => {
    renderCard(TASK, []);
    expect(screen.getByText("The role has no skills")).toBeTruthy();
  });
});

describe("TaskCard sets variables (spec 11.9, D17)", () => {
  afterEach(cleanup);

  const DOC_VARS = {
    ...DOC,
    roles: [{ key: "writer", kind: "agent" }, { key: "editor", kind: "agent" }],
    vars: [
      { key: "topics", type: "list_string", default: [], set_by: ["writer"], set_at: ["*"] },
      { key: "severity", type: "int", default: 1, set_by: ["editor"], set_at: ["*"] },
      { key: "route", type: "string", default: "", set_by: ["lead"], set_at: ["*"] },
    ],
  } as unknown as FlowDoc;

  it("lists only the variables the role of the task is allowed to set", () => {
    renderCard(TASK, SKILLS, DOC_VARS);
    expect(screen.getByText("topics")).toBeTruthy();
    expect(screen.queryByText("severity")).toBeNull();
    expect(screen.queryByText("route")).toBeNull();
  });

  it("follows the role: the same card for the other role lists the other variable", () => {
    renderCard({ ...TASK, role: "editor" } as FlowDocTask, SKILLS, DOC_VARS);
    expect(screen.getByText("severity")).toBeTruthy();
    expect(screen.queryByText("topics")).toBeNull();
  });

  it("ticking a variable puts it in Task.sets", () => {
    const onChangeTask = renderCard(TASK, SKILLS, DOC_VARS);
    fireEvent.click(screen.getByRole("checkbox", { name: "topics" }));
    expect(onChangeTask).toHaveBeenCalledWith({ ...TASK, sets: ["topics"], requires: { vars: [] } });
  });

  it("Must set before closing is offered for a variable the task sets, and writes requires.vars", () => {
    const onChangeTask = renderCard({ ...TASK, sets: ["topics"] } as FlowDocTask, SKILLS, DOC_VARS);
    fireEvent.click(screen.getByRole("checkbox", { name: "Must set before closing" }));
    expect(onChangeTask).toHaveBeenCalledWith({ ...TASK, sets: ["topics"], requires: { vars: ["topics"] } });
  });

  it("does not offer Must set before closing for a variable the task does not set", () => {
    renderCard(TASK, SKILLS, DOC_VARS);
    expect(screen.queryByRole("checkbox", { name: "Must set before closing" })).toBeNull();
  });
});

describe("TaskCard inputs (spec 11.9)", () => {
  afterEach(cleanup);

  const PREDECESSORS = [
    { stepKey: "collect", taskKey: "gather", label: "Gather", slots: ["report", "data"] },
    { stepKey: "plan", taskKey: "scope", label: "Scope", slots: [] },
  ];

  it("lists the tasks it is given, and a click puts the task in Task.inputs", () => {
    const onChangeTask = renderCard(TASK, SKILLS, DOC, { predecessorTasks: PREDECESSORS });
    expect(screen.getByText("collect.gather")).toBeTruthy();
    expect(screen.getByText("plan.scope")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: /collect\.gather/ }));
    expect(onChangeTask).toHaveBeenCalledWith({ ...TASK, inputs: [{ from: "collect.gather", required: false }] });
  });

  it("lets the person pick a file slot of the source task, and only the slots the source declares", () => {
    const task = { ...TASK, inputs: [{ from: "collect.gather", required: false }] } as FlowDocTask;
    const onChangeTask = renderCard(task, SKILLS, DOC, { predecessorTasks: PREDECESSORS });
    const slot = screen.getByRole("combobox", { name: "File slot of collect.gather" });
    expect([...(slot as HTMLSelectElement).options].map((o) => o.value)).toEqual(["", "report", "data"]);
    fireEvent.change(slot, { target: { value: "data" } });
    expect(onChangeTask).toHaveBeenCalledWith({ ...task, inputs: [{ from: "collect.gather", required: false, slot: "data" }] });
    // A source with no declared slot has no slot picker.
    expect(screen.queryByRole("combobox", { name: "File slot of plan.scope" })).toBeNull();
  });
});
