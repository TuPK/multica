// @vitest-environment jsdom

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { Outline } from "./outline";

const doc = (n: number) =>
  ({
    version: 2,
    start: "s1",
    terminals: [{ key: "done", status: "done" }],
    steps: Array.from({ length: n }, (_, i) => ({ key: `s${i + 1}`, title: `Step ${i + 1}`, kind: "tasks", tasks: [], edges: [] })),
  }) as unknown as FlowDoc;

afterEach(cleanup);

describe("Outline by keyboard", () => {
  it("Tab goes from the search box through the steps in order, and Enter or Space selects the step in focus", async () => {
    const user = userEvent.setup();
    const onSelectStep = vi.fn();
    renderWithI18n(<Outline doc={doc(3)} selectedStepKey={null} onSelectStep={onSelectStep} />);
    await user.click(screen.getByPlaceholderText("Search steps…"));
    await user.tab();
    expect(document.activeElement?.textContent).toMatch(/^Step 1/);
    await user.tab();
    expect(document.activeElement?.textContent).toMatch(/^Step 2/);
    await user.keyboard("{Enter}");
    expect(onSelectStep).toHaveBeenLastCalledWith("s2");
    await user.tab();
    await user.keyboard(" ");
    expect(onSelectStep).toHaveBeenLastCalledWith("s3");
    // Selecting does not move the focus off the step.
    expect(document.activeElement?.textContent).toMatch(/^Step 3/);
  });
});

describe("Outline step limit", () => {
  it("disables Add step at the limit it is given and says why", () => {
    renderWithI18n(<Outline doc={doc(2)} selectedStepKey={null} onSelectStep={vi.fn()} onAddStep={vi.fn()} stepLimit={2} />);
    expect((screen.getByRole("button", { name: "Add step" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Limit of 2 reached")).toBeTruthy();
  });

  it("keeps Add step enabled below the limit", () => {
    renderWithI18n(<Outline doc={doc(1)} selectedStepKey={null} onSelectStep={vi.fn()} onAddStep={vi.fn()} stepLimit={2} />);
    expect((screen.getByRole("button", { name: "Add step" }) as HTMLButtonElement).disabled).toBe(false);
    expect(screen.queryByText(/Limit of/)).toBeNull();
  });
});
