import { describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { renderWithI18n } from "../test/i18n";
import { IssueFlowStatePanel } from "./issue-flow-state-panel";
import type { IssueFlowState } from "@multica/core/types";

const mocks = vi.hoisted(() => ({
  resumeMut: vi.fn().mockResolvedValue({}),
  detachMut: vi.fn().mockResolvedValue({ detached: true }),
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
        status: "in_progress",
        outcome: null,
        assignee_id: "agent-2",
        closed_at: null,
      },
    ],
    next: [{ id: "n1", label: "Draft", to: "draft", rework: true }],
  };

  it("renders current step, round, and table of tasks with identifier and outcome", () => {
    renderWithI18n(
      <IssueFlowStatePanel
        wsId="ws-1"
        issueId="req-1"
        flow={sampleFlow}
        canManage={false}
      />
    );

    // Step & Round
    expect(screen.getByText("gate")).toBeInTheDocument();
    expect(screen.getByText(/vòng 1|round 1/i)).toBeInTheDocument();

    // Tasks table
    expect(screen.getByText("MUL-12")).toBeInTheDocument();
    expect(screen.getByText("docs_gate")).toBeInTheDocument();
    expect(screen.getByText(/rejected/i)).toBeInTheDocument();

    expect(screen.getByText("MUL-13")).toBeInTheDocument();
    expect(screen.getByText("customs_gate")).toBeInTheDocument();
  });

  it("shows stop status when stop is present", () => {
    const stoppedFlow: IssueFlowState = {
      ...sampleFlow,
      stop: 1,
    };

    renderWithI18n(
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
    const { rerender } = renderWithI18n(
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
    renderWithI18n(
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
    renderWithI18n(
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
