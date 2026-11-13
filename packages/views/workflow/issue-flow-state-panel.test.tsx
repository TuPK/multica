import { describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";
import { IssueFlowStatePanel } from "./issue-flow-state-panel";
import type { IssueFlowState } from "@multica/core/types";

const mocks = vi.hoisted(() => ({
  resumeMut: vi.fn().mockResolvedValue({}),
  detachMut: vi.fn().mockResolvedValue({ detached: true }),
  updateIssueMut: vi.fn().mockResolvedValue({}),
}));

vi.mock("@multica/core/issues/mutations", () => ({
  useUpdateIssue: () => ({
    mutateAsync: mocks.updateIssueMut,
    isPending: false,
  }),
}));

vi.mock("@multica/core/issue-workflows", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/issue-workflows")>();
  return {
    ...actual,
    useResumeFlow: () => ({
      mutateAsync: mocks.resumeMut,
      isPending: false,
    }),
    useDetachFlow: () => ({
      mutateAsync: mocks.detachMut,
      isPending: false,
    }),
  };
});

// The panel refreshes the flow through the query client, so every render gets one.
function renderPanel(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: React.ReactElement) => (
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  );
  const result = renderWithI18n(wrap(ui));
  return { ...result, rerender: (next: React.ReactElement) => result.rerender(wrap(next)) };
}

describe("IssueFlowStatePanel", () => {
  const sampleFlow: IssueFlowState = {
    managed: true,
    role: "request",
    workflow_id: "wf-1",
    workflow_name: "Import & Export Desk · a1b2c3",
    step: "gate",
    rework: 1,
    stop: null,
    switches: { route: "shipment_dossier", customs_lookup: true },
    auto: false,
    entered_at: "2026-11-13T00:00:00Z",
    tasks: [
      {
        issue_id: "task-1",
        identifier: "MUL-12",
        step: "gate",
        task: "docs_gate",
        round: 1,
        kind: "gate",
        status: "done",
        outcome: "rejected",
        assignee_id: "agent-1",
        closed_at: "2026-11-13T00:05:00Z",
      },
      {
        issue_id: "task-2",
        identifier: "MUL-13",
        step: "gate",
        task: "customs_gate",
        round: 1,
        kind: "gate",
        status: "in_progress",
        outcome: null,
        assignee_id: "agent-2",
        closed_at: null,
      },
    ],
    next: [{ id: "n1", label: "Draft", to: "draft", rework: true }],
  };

  it("renders current step, round, and the list of tasks with identifier and outcome", () => {
    renderPanel(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={sampleFlow}
        canManage={false}
      />
    );

    // Step & Round
    expect(screen.getByText("gate")).toBeInTheDocument();
    expect(screen.getAllByText(/vòng 1|round 1/i).length).toBeGreaterThan(0);

    // Tasks table
    expect(screen.getByText("MUL-12")).toBeInTheDocument();
    expect(screen.getByText("docs_gate")).toBeInTheDocument();
    expect(screen.getByText(/rejected/i)).toBeInTheDocument();

    expect(screen.getByText("MUL-13")).toBeInTheDocument();
    expect(screen.getByText("customs_gate")).toBeInTheDocument();
  });

  it("records a verdict on an open gate task as done with the outcome", async () => {
    const openGate: IssueFlowState = {
      ...sampleFlow,
      tasks: [
        {
          issue_id: "task-3",
          identifier: "MUL-14",
          step: "gate",
          task: "handover",
          round: 1,
          kind: "gate",
          status: "todo",
          outcome: null,
          assignee_id: "agent-3",
          closed_at: null,
        },
      ],
    };
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={openGate} canManage={false} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() =>
      expect(mocks.updateIssueMut).toHaveBeenCalledWith({
        id: "task-3",
        status: "done",
        outcome: "approved",
      }),
    );
  });

  it("offers no verdict on a work task or a closed gate", () => {
    const openWork: IssueFlowState = {
      ...sampleFlow,
      tasks: [
        {
          issue_id: "task-4",
          identifier: "MUL-15",
          step: "collect",
          task: "docs",
          round: 0,
          kind: "work",
          status: "todo",
          outcome: null,
          assignee_id: "agent-4",
          closed_at: null,
        },
      ],
    };
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={openWork} canManage={false} />,
    );

    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
  });

  it("stacks each task in its own block so status and verdict stay visible in a 225px sidebar", () => {
    // jsdom cannot measure layout. A table needs about 365px and hid the status, the outcome and the
    // verdict buttons behind a horizontal scroll; a block per task wraps instead.
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={sampleFlow} canManage={false} />,
    );

    expect(screen.queryByRole("table")).toBeNull();
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("MUL-12");
    expect(items[0]).toHaveTextContent("docs_gate");
    expect(items[0]).toHaveTextContent("done");
    expect(items[0]).toHaveTextContent(/rejected/i);
    expect(items[1]).toHaveTextContent("MUL-13");
    expect(items[1]).toHaveTextContent("in_progress");
    expect(within(items[1]!).getByRole("button", { name: "Approve" })).toBeInTheDocument();
  });

  it("marks a closed work task without an outcome as done, not pending", () => {
    const closedWork: IssueFlowState = {
      ...sampleFlow,
      tasks: [
        {
          issue_id: "task-5",
          identifier: "MUL-16",
          step: "collect",
          task: "docs",
          round: 0,
          kind: "work",
          status: "done",
          outcome: null,
          assignee_id: "agent-4",
          closed_at: "2026-11-13T00:05:00Z",
        },
      ],
    };
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={closedWork} canManage={false} />,
    );

    const item = screen.getByRole("listitem");
    expect(item).toHaveTextContent(/done/i);
    expect(item).not.toHaveTextContent(/pending/i);
  });

  it("shows stop status when stop is present", () => {
    const stoppedFlow: IssueFlowState = {
      ...sampleFlow,
      stop: 1,
    };

    renderPanel(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={stoppedFlow}
        canManage={false}
      />
    );

    expect(screen.getByText(/dừng 1|stop 1|blocked/i)).toBeInTheDocument();
  });

  it("shows Resume and Detach buttons only when canManage is true", () => {
    const { rerender } = renderPanel(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={sampleFlow}
        canManage={false}
      />
    );

    expect(screen.queryByRole("button", { name: /resume/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /detach/i })).not.toBeInTheDocument();

    rerender(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={sampleFlow}
        canManage={true}
      />
    );

    expect(screen.getByRole("button", { name: /resume/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /detach/i })).toBeInTheDocument();
  });

  it("clicking Detach opens confirmation dialog and calls detach", async () => {
    renderPanel(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={sampleFlow}
        canManage={true}
      />
    );

    const detachBtn = screen.getByRole("button", { name: /detach/i });
    fireEvent.click(detachBtn);

    // Confirm dialog
    const confirmBtn = screen.getByRole("button", { name: /xác nhận tách|confirm detach/i });
    expect(confirmBtn).toBeInTheDocument();

    fireEvent.click(confirmBtn);
    await waitFor(() => {
      expect(mocks.detachMut).toHaveBeenCalled();
    });
  });

  it("clicking Resume calls useResumeFlow", async () => {
    renderPanel(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={sampleFlow}
        canManage={true}
      />
    );

    const resumeBtn = screen.getByRole("button", { name: /resume/i });
    fireEvent.click(resumeBtn);

    await waitFor(() => {
      expect(mocks.resumeMut).toHaveBeenCalled();
    });
  });
});
