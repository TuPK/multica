// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { DesignerCanvas } from "./designer-canvas";
import type { IssueWorkflowStep } from "../types";

vi.mock("@multica/core/issue-statuses/hooks", () => {
  // The real hook returns a stable catalog between renders; so does this mock.
  const catalog = {
    statuses: [
      { id: "s-todo", key: "todo", name: "To Do" },
      { id: "s-inp", key: "in_progress", name: "In Progress" },
      { id: "s-done", key: "done", name: "Done" },
    ],
    entryOf: (k: string) => ({ id: `s-${k}`, key: k, name: k }),
  };
  return { useIssueStatuses: () => catalog };
});

vi.mock("../../issues/utils/status-label", () => ({
  useStatusLabel: () => (key: string) => {
    const map: Record<string, string> = {
      todo: "To Do",
      in_progress: "In Progress",
      done: "Done",
    };
    return map[key] || key;
  },
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"], queryFn: () => [] }),
  agentListOptions: () => ({ queryKey: ["agents"], queryFn: () => [] }),
}));

describe("DesignerCanvas", () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  const sampleSteps: IssueWorkflowStep[] = [
    {
      status_key: "todo",
      handler: { type: "none" },
      instructions: "",
      transitions: [{ id: "t-1", label: "Start", when: null, to_status_key: "in_progress" }],
      layout: { x: 50, y: 50 },
    },
    {
      status_key: "in_progress",
      handler: { type: "none" },
      instructions: "",
      transitions: [{ id: "t-2", label: "Complete", when: null, to_status_key: "done" }],
      layout: { x: 350, y: 50 },
    },
    {
      status_key: "done",
      handler: { type: "none" },
      instructions: "",
      transitions: [],
      layout: { x: 650, y: 50 },
    },
  ];

  it("renders status nodes on the canvas", () => {
    const onSelectStep = vi.fn();
    const onUpdateSteps = vi.fn();
    const onEditTransition = vi.fn();
    const onAddTransition = vi.fn();

    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <div style={{ width: 1000, height: 800 }}>
          <DesignerCanvas
            steps={sampleSteps}
            wsId="ws-1"
            onSelectStep={onSelectStep}
            onUpdateSteps={onUpdateSteps}
            onEditTransition={onEditTransition}
            onAddTransition={onAddTransition}
          />
        </div>
      </QueryClientProvider>,
    );

    expect(screen.getByTestId("status-node-todo")).toBeTruthy();
    expect(screen.getByTestId("status-node-in_progress")).toBeTruthy();
    expect(screen.getByTestId("status-node-done")).toBeTruthy();

    fireEvent.click(screen.getByTestId("status-node-todo"));
    expect(onSelectStep).toHaveBeenCalledWith("todo");
  });
});
