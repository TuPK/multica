// @vitest-environment jsdom

import React from "react";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { LoopsPanel } from "./loops-panel";
import { RolesPanel } from "./roles-panel";
import { StepInspector } from "./step-inspector";
import { VarsPanel } from "./vars-panel";
import { VerdictsPanel } from "./verdicts-panel";

// Every list of the editor is capped by the numbers the server gives in `capabilities.limits`. At the cap the
// button that adds one is disabled and a line says which limit it is.

const DOC = {
  version: 2,
  start: "a",
  roles: [{ key: "r1", kind: "agent" }, { key: "r2", kind: "agent" }],
  vars: [{ key: "v1", type: "bool", default: false }, { key: "v2", type: "bool", default: false }],
  verdicts: [{ key: "w1", verdicts: [] }, { key: "w2", verdicts: [] }],
  loops: [{ key: "l1", header: "a", max: 2 }, { key: "l2", header: "a", max: 2 }],
  steps: [{ key: "a", title: "A", kind: "tasks", tasks: [{ key: "t1", title: "T1", kind: "work", role: "r1" }, { key: "t2", title: "T2", kind: "work", role: "r1" }], edges: [] }],
  terminals: [],
} as unknown as FlowDoc;

vi.mock("@multica/core/paths", () => ({ useWorkspacePaths: () => ({ squadDetail: (id: string) => `/acme/squads/${id}` }) }));
vi.mock("../../navigation", () => ({
  AppLink: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));

afterEach(cleanup);

const LIMIT_TEXT = "Limit of 2 reached";

describe("limits from capabilities", () => {
  it("roles", () => {
    renderWithI18n(<RolesPanel doc={DOC} onChangeRoles={vi.fn()} roleLimit={2} />);
    expect(screen.getByText("Role limit of 2 reached")).toBeTruthy();
  });

  it("variables", () => {
    renderWithI18n(<VarsPanel doc={DOC} onChangeVars={vi.fn()} varLimit={2} />);
    expect(screen.getByText("Variable limit of 2 reached")).toBeTruthy();
  });

  it("verdict sets", () => {
    renderWithI18n(<VerdictsPanel doc={DOC} onChangeVerdicts={vi.fn()} verdictLimit={2} />);
    expect((screen.getByRole("button", { name: /Add/ }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(LIMIT_TEXT)).toBeTruthy();
  });

  it("loops", () => {
    renderWithI18n(<LoopsPanel doc={DOC} onChangeLoops={vi.fn()} loopLimit={2} />);
    expect((screen.getByRole("button", { name: /Add/ }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(LIMIT_TEXT)).toBeTruthy();
  });

  it("tasks of a step", () => {
    renderWithI18n(<StepInspector step={DOC.steps[0]!} onChangeStep={vi.fn()} doc={DOC} taskLimit={2} />);
    expect((screen.getByRole("button", { name: "Add task" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(LIMIT_TEXT)).toBeTruthy();
  });

  it("below the limit nothing is disabled and no limit line shows", () => {
    renderWithI18n(<StepInspector step={DOC.steps[0]!} onChangeStep={vi.fn()} doc={DOC} taskLimit={12} />);
    expect((screen.getByRole("button", { name: "Add task" }) as HTMLButtonElement).disabled).toBe(false);
    expect(screen.queryByText(/Limit of/)).toBeNull();
  });
});
