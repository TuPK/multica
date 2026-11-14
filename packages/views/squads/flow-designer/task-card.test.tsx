// @vitest-environment jsdom

import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc, FlowDocTask, Skill } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { TaskCard } from "./task-card";

const DOC = { version: 2, start: "a", roles: [{ key: "writer", kind: "agent" }], steps: [], terminals: [], policy: {} } as unknown as FlowDoc;
const SKILLS = [{ name: "outline-first" }, { name: "tone-check" }] as unknown as Skill[];

function renderCard(task: FlowDocTask, skills: Skill[] = SKILLS) {
  const onChangeTask = vi.fn();
  renderWithI18n(<TaskCard task={task} onChangeTask={onChangeTask} onDeleteTask={vi.fn()} doc={DOC} currentStepKey="write" squadSkills={skills} />);
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

  it("says the squad has no skills when it has none", () => {
    renderCard(TASK, []);
    expect(screen.getByText("The squad has no skills")).toBeTruthy();
  });
});
