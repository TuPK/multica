import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";
import { IssueFlowStatePanel } from "./issue-flow-state-panel";
import type { IssueFlowState } from "@multica/core/types";

const mocks = vi.hoisted(() => ({
  resumeMut: vi.fn().mockResolvedValue({}),
  detachMut: vi.fn().mockResolvedValue({ detached: true }),
  exitMut: vi.fn().mockResolvedValue({}),
  closeMut: vi.fn().mockResolvedValue({}),
  userId: "user-1" as string | null,
  updateIssueMut: vi.fn().mockResolvedValue({}),
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (select: (state: { user: { id: string } | null }) => unknown) =>
    select({ user: mocks.userId ? { id: mocks.userId } : null }),
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
    useExitIssueFlow: () => ({
      mutateAsync: mocks.exitMut,
      isPending: false,
    }),
    useCloseIssueFlowTask: () => ({
      mutateAsync: mocks.closeMut,
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
      stop: { code: "grounds_missing", legacy_no: 1, from: "gate", edge: "gate_blocked", at: "" },
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

  // A stopped request whose stop has a drafting step offers it on the Resume button, so a draft that
  // is at fault can be sent back to drafting in a new round. The default Resume still sends no step.
  const stoppedAtGate: IssueFlowState = { ...sampleFlow, step: "blocked", stop: { code: "grounds_missing", legacy_no: 1, from: "gate", edge: "gate_blocked", at: "" }, rework_step: "draft" };

  it("sends the request back to drafting from the Resume choices", async () => {
    const user = userEvent.setup();
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={stoppedAtGate} canManage={true} />
    );

    await user.click(screen.getByRole("button", { name: "Choose the step" }));
    await user.click(await screen.findByRole("menuitem", { name: "Back to drafting" }));

    await waitFor(() => {
      expect(mocks.resumeMut).toHaveBeenLastCalledWith({ to_step: "draft" });
    });
  });

  it("keeps the default Resume without a step", async () => {
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={stoppedAtGate} canManage={true} />
    );

    fireEvent.click(screen.getByRole("button", { name: "Resume" }));

    await waitFor(() => {
      expect(mocks.resumeMut).toHaveBeenLastCalledWith({});
    });
  });

  it("offers no drafting step when the state names none", () => {
    renderPanel(
      <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={{ ...stoppedAtGate, rework_step: "" }} canManage={true} />
    );

    expect(screen.queryByRole("button", { name: "Choose the step" })).not.toBeInTheDocument();
  });

  // A step of kind manual waits for a person to take one of its exits (spec 4.7, 11.6).
  it("takes a manual exit through the exit route, by the id of the edge", async () => {
    const manual: IssueFlowState = {
      ...sampleFlow,
      next: [
        { id: "ship", label: "Ship it", to: "done", rework: false, trigger: "manual" },
        { id: "auto_edge", label: "Auto", to: "x", rework: false, trigger: "auto" },
      ],
    };
    renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={manual} canManage={true} />);

    expect(screen.queryByRole("button", { name: "Auto" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Ship it" }));

    await waitFor(() => expect(mocks.exitMut).toHaveBeenCalledWith("ship"));
  });

  // Work a person cancelled is reopened by a resume unless the resume says to go on without it (H3).
  it("resumes without the cancelled work when a work task was cancelled", async () => {
    const user = userEvent.setup();
    const cancelled: IssueFlowState = {
      ...stoppedAtGate,
      rework_step: "",
      tasks: [{ ...stoppedAtGate.tasks[0]!, kind: "work", status: "cancelled", task: "collect", closed_at: null }],
    };
    renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={cancelled} canManage={true} />);

    await user.click(screen.getByRole("button", { name: "Choose the step" }));
    await user.click(await screen.findByRole("menuitem", { name: "Resume without the cancelled tasks" }));

    await waitFor(() => expect(mocks.resumeMut).toHaveBeenLastCalledWith({ skip_cancelled: true }));
  });

  it("does not offer to skip cancelled tasks when none were cancelled", () => {
    renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={{ ...stoppedAtGate, rework_step: "" }} canManage={true} />);
    expect(screen.queryByRole("button", { name: "Choose the step" })).not.toBeInTheDocument();
  });

  // The buttons of a task are the verdicts of its own set (spec 2.3, 11.6), not two fixed ones.
  describe("a trial run", () => {
    it("shows the Trial badge next to the workflow name, and none for an ordinary request", () => {
      const { rerender } = renderPanel(
        <IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={{ ...sampleFlow, trial: true }} canManage={false} />,
      );
      expect(screen.getByText("Trial")).toBeInTheDocument();
      expect(screen.getByText(sampleFlow.workflow_name).parentElement).toContainElement(screen.getByText("Trial"));

      rerender(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={{ ...sampleFlow, trial: false }} canManage={false} />);
      expect(screen.queryByText("Trial")).toBeNull();
      // A server that does not send the field is an ordinary request.
      rerender(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={sampleFlow} canManage={false} />);
      expect(screen.queryByText("Trial")).toBeNull();
    });
  });

  describe("verdicts of the task's own set", () => {
    beforeEach(() => {
      mocks.updateIssueMut.mockClear();
      mocks.closeMut.mockClear();
    });

    const triage = (over: Partial<IssueFlowState["tasks"][number]> = {}): IssueFlowState => ({
      ...sampleFlow,
      tasks: [
        {
          issue_id: "task-9",
          identifier: "MUL-20",
          step: "review",
          task: "triage",
          round: 0,
          kind: "gate",
          status: "todo",
          outcome: null,
          assignee_id: "user-1",
          closed_at: null,
          verdicts: [
            { key: "ship", class: "pass" },
            { key: "fix", class: "fail" },
            { key: "defer", class: "neutral" },
          ],
          ...over,
        },
      ],
    });

    it("shows the custom verdict keys to the assignee, and closes with the key chosen", async () => {
      mocks.userId = "user-1";
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={triage()} canManage={false} />);

      expect(screen.getByRole("button", { name: "ship" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "fix" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "defer" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();

      fireEvent.click(screen.getByRole("button", { name: "fix" }));
      await waitFor(() => expect(mocks.updateIssueMut).toHaveBeenCalledWith({ id: "task-9", status: "done", outcome: "fix" }));
      expect(mocks.closeMut).not.toHaveBeenCalled();
    });

    it("lets a manager who is not the assignee close it on behalf through the close route, with a verdict of the set", async () => {
      const user = userEvent.setup();
      mocks.userId = "manager-1";
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={triage()} canManage={true} />);

      expect(screen.queryByRole("button", { name: "ship" })).toBeNull();
      await user.click(screen.getByRole("button", { name: /close on behalf/i }));
      await user.click(await screen.findByRole("menuitem", { name: "ship" }));

      await waitFor(() => expect(mocks.closeMut).toHaveBeenCalledWith({ taskId: "task-9", verdict: "ship" }));
      expect(mocks.updateIssueMut).not.toHaveBeenCalled();
    });

    it("gives the buttons of the built-in set when an older server sends none", () => {
      mocks.userId = "user-1";
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={triage({ verdicts: [] })} canManage={false} />);

      expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Reject" })).toBeInTheDocument();
    });

    it("shows nothing to someone who may not close the task", () => {
      mocks.userId = "other";
      const flow = { ...triage(), viewer: { can_manage: false, closable_task_ids: [] } };
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={flow} canManage={false} />);

      expect(screen.queryByRole("button", { name: "ship" })).toBeNull();
    });
  });

  // Spec 4.7: a stall caused by agent work nobody has seen is resumed only on the manager's word.
  describe("resuming a request stalled on unseen work", () => {
    const unseen: IssueFlowState = {
      ...sampleFlow,
      unreviewed: true,
      stop: { code: "stalled", legacy_no: 0, from: "handover", edge: "", at: "" },
    };

    beforeEach(() => mocks.resumeMut.mockClear());

    it("asks for the acknowledgement first and sends it only once it is ticked", async () => {
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={unseen} canManage={true} />);

      fireEvent.click(screen.getByRole("button", { name: "Resume" }));
      const dialog = await screen.findByRole("dialog");
      expect(mocks.resumeMut).not.toHaveBeenCalled();
      const confirm = within(dialog).getByRole("button", { name: "Resume" });
      expect(confirm).toBeDisabled();

      fireEvent.click(within(dialog).getByRole("checkbox"));
      fireEvent.click(confirm);

      await waitFor(() => expect(mocks.resumeMut).toHaveBeenCalledWith({ acknowledge_skip_checkpoint: true }));
    });

    it("does not ask when nothing is unseen, and never sends the acknowledgement then", async () => {
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={{ ...unseen, unreviewed: false }} canManage={true} />);

      fireEvent.click(screen.getByRole("button", { name: "Resume" }));

      await waitFor(() => expect(mocks.resumeMut).toHaveBeenCalledWith({}));
      expect(screen.queryByRole("dialog")).toBeNull();
    });

    it("asks when the server asks, though the state did not say so", async () => {
      mocks.resumeMut.mockRejectedValueOnce(Object.assign(new Error("ack"), { body: { code: "flow_checkpoint_ack_required" } }));
      renderPanel(<IssueFlowStatePanel wsId="ws-1" issueId="req-1" flow={{ ...unseen, unreviewed: false }} canManage={true} />);

      fireEvent.click(screen.getByRole("button", { name: "Resume" }));

      expect(await screen.findByRole("dialog")).toBeInTheDocument();
    });
  });
});
