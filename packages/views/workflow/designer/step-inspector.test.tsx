// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { StepInspector, type StepInspectorProps } from "./step-inspector";
import type { IssueWorkflowStep } from "../types";

vi.mock("@multica/core/issue-statuses/hooks", () => ({
  useIssueStatuses: () => ({
    statuses: [
      { id: "s-todo", key: "todo", name: "To Do" },
      { id: "s-inp", key: "in_progress", name: "In Progress" },
      { id: "s-done", key: "done", name: "Done" },
      { id: "s-can", key: "cancelled", name: "Cancelled" },
    ],
    entryOf: (k: string) => ({ id: `s-${k}`, key: k, name: k }),
  }),
}));

vi.mock("../../issues/utils/status-label", () => ({
  useStatusLabel: () => (key: string) => {
    const map: Record<string, string> = {
      todo: "To Do",
      in_progress: "In Progress",
      done: "Done",
      cancelled: "Cancelled",
    };
    return map[key] || key;
  },
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({
    queryKey: ["members"],
    queryFn: () => [{ id: "m-1", name: "Alice", email: "alice@example.com" }],
  }),
  agentListOptions: () => ({
    queryKey: ["agents"],
    queryFn: () => [{ id: "ag-1", name: "Worker Bot" }],
  }),
}));

describe("StepInspector", () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  const sampleSteps: IssueWorkflowStep[] = [
    {
      status_key: "todo",
      handler: { type: "none" },
      instructions: "Check requirements",
      transitions: [{ id: "t-1", label: "Start", when: null, to_status_key: "in_progress" }],
    },
    {
      status_key: "in_progress",
      handler: { type: "none" },
      role_hint: "lead_dev",
      instructions: "Implement task",
      transitions: [{ id: "t-2", label: "Complete", when: null, to_status_key: "done" }],
    },
    {
      status_key: "done",
      handler: { type: "none" },
      instructions: "",
      transitions: [],
    },
    {
      status_key: "cancelled",
      handler: { type: "none" },
      instructions: "",
      transitions: [],
    },
  ];

  const defaultProps: StepInspectorProps = {
    step: sampleSteps[0] ?? null,
    steps: sampleSteps,
    wsId: "ws-1",
    onUpdateStep: vi.fn(),
    onMoveStepUp: vi.fn(),
    onMoveStepDown: vi.fn(),
    onRemoveStep: vi.fn(),
    onAddStep: vi.fn(),
    onSelectStep: vi.fn(),
    onAddTransition: vi.fn(),
    onEditTransition: vi.fn(),
    onDeleteTransition: vi.fn(),
    onClose: vi.fn(),
  };

  const renderInspector = (props: Partial<StepInspectorProps> = {}) => {
    const finalStep: IssueWorkflowStep | null = "step" in props ? (props.step ?? null) : defaultProps.step;
    return renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <StepInspector {...defaultProps} {...props} step={finalStep} />
      </QueryClientProvider>,
    );
  };

  it("renders step details, instructions, and transition list", () => {
    renderInspector();

    expect(screen.getAllByText("To Do").length).toBeGreaterThan(0);
    expect(screen.getByDisplayValue("Check requirements")).toBeTruthy();
    expect(screen.getByText("Start")).toBeTruthy();
    expect(screen.getAllByText("In Progress").length).toBeGreaterThan(0);
  });

  it("displays role hint when present on step", () => {
    renderInspector({ step: sampleSteps[1] });

    expect(screen.getByText("Role: lead_dev")).toBeTruthy();
    expect(screen.getByDisplayValue("Implement task")).toBeTruthy();
  });

  it("calls onUpdateStep when instructions change", () => {
    const onUpdateStep = vi.fn();
    renderInspector({ onUpdateStep });

    const textarea = screen.getByDisplayValue("Check requirements");
    fireEvent.change(textarea, { target: { value: "New updated requirements" } });

    expect(onUpdateStep).toHaveBeenCalledWith(
      expect.objectContaining({
        status_key: "todo",
        instructions: "New updated requirements",
      }),
    );
  });

  it("provides keyboard reordering path with Move Up and Move Down", () => {
    const onMoveStepUp = vi.fn();
    const onMoveStepDown = vi.fn();
    renderInspector({ step: sampleSteps[1], onMoveStepUp, onMoveStepDown });

    const moveUpBtn = screen.getAllByRole("button", { name: "Move up" })[0]!;
    const moveDownBtn = screen.getAllByRole("button", { name: "Move down" })[0]!;

    fireEvent.click(moveUpBtn);
    expect(onMoveStepUp).toHaveBeenCalledWith("in_progress");

    fireEvent.click(moveDownBtn);
    expect(onMoveStepDown).toHaveBeenCalledWith("in_progress");
  });

  it("allows adding and editing transitions", () => {
    const onAddTransition = vi.fn();
    const onEditTransition = vi.fn();
    const onDeleteTransition = vi.fn();
    renderInspector({ onAddTransition, onEditTransition, onDeleteTransition });

    fireEvent.click(screen.getByRole("button", { name: "Add transition" }));
    expect(onAddTransition).toHaveBeenCalledWith("todo");

    fireEvent.click(screen.getByRole("button", { name: "Edit transition" }));
    expect(onEditTransition).toHaveBeenCalledWith(
      "todo",
      expect.objectContaining({ id: "t-1", label: "Start" }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete transition" }));
    expect(onDeleteTransition).toHaveBeenCalledWith("todo", "t-1");
  });

  it("renders empty state with steps keyboard picker when no step is selected", () => {
    const onSelectStep = vi.fn();
    const onAddStep = vi.fn();
    renderInspector({ step: null, onSelectStep, onAddStep });

    expect(screen.getByText("Select a step to inspect its handler and transitions.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add step" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Add step" }));
    expect(onAddStep).toHaveBeenCalledTimes(1);

    // Clicking step in the keyboard list selects it
    const todoBtn = screen.getAllByRole("button", { name: /to do/i })[0]!;
    fireEvent.click(todoBtn);
    expect(onSelectStep).toHaveBeenCalledWith("todo");
  });
});
