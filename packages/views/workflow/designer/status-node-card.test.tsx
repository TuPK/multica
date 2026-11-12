// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { renderWithI18n } from "../../test/i18n";
import { StatusNodeCard } from "./status-node-card";
import type { IssueWorkflowStep } from "../types";

vi.mock("@multica/core/issue-statuses/hooks", () => ({
  useIssueStatuses: () => ({
    statuses: [
      { id: "s-todo", key: "todo", name: "To Do" },
      { id: "s-done", key: "done", name: "Done" },
    ],
    entryOf: (k: string) => ({ id: `s-${k}`, key: k, name: k }),
  }),
}));

vi.mock("../../issues/utils/status-label", () => ({
  useStatusLabel: () => (key: string) => {
    if (key === "todo") return "To Do";
    if (key === "done") return "Done";
    return key;
  },
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({
    queryKey: ["members"],
    queryFn: () => [
      { id: "m-1", name: "Alice Developer", email: "alice@example.com" },
    ],
  }),
  agentListOptions: () => ({
    queryKey: ["agents"],
    queryFn: () => [
      { id: "ag-1", name: "Claude Worker" },
    ],
  }),
}));

describe("StatusNodeCard", () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  const nodeTypes = { statusNode: StatusNodeCard };

  const renderCard = (
    step: IssueWorkflowStep,
    onSelect = vi.fn(),
    statusDef = { id: `s-${step.status_key}`, key: step.status_key, name: step.status_key, category: "unstarted" as const, position: 0 },
  ) => {
    const nodes = [
      {
        id: step.status_key,
        type: "statusNode" as const,
        position: { x: 0, y: 0 },
        data: {
          step,
          wsId: "ws-1",
          statusDef,
          onSelect,
        },
      },
    ];

    return renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <ReactFlowProvider>
          <div style={{ width: 800, height: 600 }}>
            <ReactFlow nodes={nodes} nodeTypes={nodeTypes} />
          </div>
        </ReactFlowProvider>
      </QueryClientProvider>,
    );
  };

  it("renders status name and transition count", () => {
    const step: IssueWorkflowStep = {
      status_key: "todo",
      handler: { type: "none" },
      instructions: "",
      transitions: [{ id: "t1", label: "Start", when: null, to_status_key: "done" }],
    };

    renderCard(step);

    expect(screen.getByText("To Do")).toBeTruthy();
    expect(screen.getByText("Transitions")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
    expect(screen.getByText("Keep assignee")).toBeTruthy();
  });

  it("renders role hint chip when unassigned with role_hint", () => {
    const step: IssueWorkflowStep = {
      status_key: "todo",
      handler: { type: "none" },
      role_hint: "lead_dev",
      instructions: "",
    };

    renderCard(step);

    const chip = screen.getByTestId("role-hint-chip");
    expect(chip).toBeTruthy();
    expect(chip.textContent).toBe("lead_dev");
  });

  it("renders will_be_created badge when step has new_status", () => {
    const step: IssueWorkflowStep = {
      status_key: "scouting",
      new_status: {
        name: "Scouting",
        category: "started",
        color: "#10b981",
      },
      handler: { type: "none" },
      instructions: "",
    };

    renderCard(step);

    expect(screen.getByText("Scouting")).toBeTruthy();
    expect(screen.getByText("Will be created")).toBeTruthy();
  });

  it("triggers onSelect callback when clicked", () => {
    const onSelect = vi.fn();
    const step: IssueWorkflowStep = {
      status_key: "todo",
      handler: { type: "none" },
      instructions: "",
    };

    renderCard(step, onSelect);

    const card = screen.getByTestId("status-node-todo");
    fireEvent.click(card);
    expect(onSelect).toHaveBeenCalledTimes(1);
  });
});
