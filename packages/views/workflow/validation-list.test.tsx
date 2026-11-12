// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { ValidationList } from "./designer/validation-list";
import type { ValidationIssue } from "./types";

describe("ValidationList", () => {
  it("renders valid indicator when there are no issues", () => {
    renderWithI18n(<ValidationList issues={[]} />);
    expect(screen.getByText("Workflow is valid")).toBeTruthy();
  });

  it("renders error count when errors exist", () => {
    const issues: ValidationIssue[] = [
      {
        id: "missing-done",
        severity: "error",
        message: { key: "missing_required_status", values: { status: "done" } },
        step_key: "done",
      },
    ];

    renderWithI18n(<ValidationList issues={issues} />);
    expect(screen.getByText(/1 Errors/i)).toBeTruthy();
  });

  it("renders warnings when warnings exist", () => {
    const issues: ValidationIssue[] = [
      {
        id: "unreachable-terminal-todo",
        severity: "warning",
        message: { key: "unreachable_terminal", values: { step: "todo" } },
        step_key: "todo",
      },
    ];

    renderWithI18n(<ValidationList issues={issues} />);
    expect(screen.getByText(/1 Warnings/i)).toBeTruthy();
  });

  it("clicking issue inside popover calls onSelectStep", () => {
    const issues: ValidationIssue[] = [
      {
        id: "err-1",
        severity: "error",
        message: { key: "label_required", values: { step: "in_progress" } },
        step_key: "in_progress",
      },
    ];
    const onSelectStep = vi.fn();

    renderWithI18n(
      <ValidationList issues={issues} onSelectStep={onSelectStep} />,
    );

    // Open popover by clicking the trigger
    const trigger = screen.getByRole("button");
    fireEvent.click(trigger);

    // Click the issue row
    const issueRow = screen.getByText("A branch on 'in_progress' needs a label.");
    fireEvent.click(issueRow);

    expect(onSelectStep).toHaveBeenCalledWith("in_progress");
  });
});
