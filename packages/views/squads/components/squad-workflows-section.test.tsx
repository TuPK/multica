import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import type { IssueWorkflow, Squad, SquadWorkflowsResponse } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { SquadWorkflowsSection } from "./squad-workflows-section";

const mocks = vi.hoisted(() => ({
  data: undefined as SquadWorkflowsResponse | undefined,
  isPending: false,
  isError: false,
  refetch: vi.fn(),
  setActiveWorkflow: vi.fn(),
  setFlowAuto: vi.fn(),
}));

vi.mock("@multica/core/issue-workflows", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/issue-workflows")>();
  return {
    ...actual,
    squadWorkflowsOptions: () => ({ queryKey: ["squad-workflows", "ws-1", "sq-1"] }),
    useSetSquadActiveWorkflow: () => ({
      mutateAsync: mocks.setActiveWorkflow,
      isPending: false,
    }),
    useSetFlowAuto: () => ({
      mutateAsync: mocks.setFlowAuto,
      isPending: false,
    }),
  };
});

vi.mock("@tanstack/react-query", () => ({
  queryOptions: (opts: unknown) => opts,
  useQuery: () => ({
    data: mocks.data,
    isPending: mocks.isPending,
    isError: mocks.isError,
    refetch: mocks.refetch,
  }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  useMutation: ({ mutationFn }: { mutationFn: (...args: any[]) => any }) => ({
    mutateAsync: mutationFn,
    isPending: false,
  }),
}));

describe("SquadWorkflowsSection", () => {
  afterEach(() => {
    cleanup();
  });

  const baseSquad = {
    id: "sq-1",
    name: "Trade Squad",
    workflow_count: 1,
    active_workflow_id: "wf-1",
    active_workflow_name: "Import & Export Desk · a1b2c3",
    flow_enforced: true,
  } as unknown as Squad;

  it("renders section title 'Workflows'", () => {
    mocks.data = { active_workflow_id: null, workflows: [], total: 0 };
    mocks.isPending = false;
    mocks.isError = false;

    renderWithI18n(<SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={false} />);
    expect(screen.getByText("Workflows")).toBeInTheDocument();
  });

  it("renders empty state when there are no workflows", () => {
    mocks.data = { active_workflow_id: null, workflows: [], total: 0 };
    mocks.isPending = false;
    mocks.isError = false;

    renderWithI18n(<SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={false} />);
    expect(screen.getByText(/chưa có workflow|no workflows yet/i)).toBeInTheDocument();
  });

  it("renders error state with retry button when query fails", () => {
    mocks.data = undefined;
    mocks.isPending = false;
    mocks.isError = true;

    renderWithI18n(<SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={false} />);
    const retryBtn = screen.getByRole("button", { name: /try again|thử lại|retry/i });
    expect(retryBtn).toBeInTheDocument();
    fireEvent.click(retryBtn);
    expect(mocks.refetch).toHaveBeenCalled();
  });

  it("renders list of workflows with active badge, template ref, steps & tasks count, and open flow requests", () => {
    const wf1: IssueWorkflow = {
      id: "wf-1",
      name: "Import & Export Desk · a1b2c3",
      template_ref: "kv_trade@a1b2c3",
      flow_auto: true,
      open_flow_requests: 4,
      steps: [
        {
          status_key: "collect",
          tasks: [
            { key: "docs_reader", title: "trade_docs_reader", kind: "work", handler: { type: "agent", id: "agent-trade_docs_reader" }, instructions: "", requires: [] },
            { key: "customs_research", title: "trade_customs_research", kind: "work", handler: { type: "agent", id: "agent-trade_customs_research" }, instructions: "", requires: [] },
          ],
        },
        {
          status_key: "draft",
          tasks: [
            { key: "crosscheck_drafter", title: "trade_crosscheck_drafter", kind: "work", handler: { type: "agent", id: "agent-trade_crosscheck_drafter" }, instructions: "", requires: [] },
          ],
        },
        {
          status_key: "gate",
          tasks: [
            { key: "docs_gate", title: "trade_docs_gate", kind: "gate", handler: { type: "agent", id: "agent-trade_docs_gate" }, instructions: "", requires: [] },
            { key: "customs_gate", title: "trade_customs_gate", kind: "gate", handler: { type: "agent", id: "agent-trade_customs_gate" }, instructions: "", requires: [] },
          ],
        },
        {
          status_key: "handover",
          tasks: [
            { key: "handover", title: "trade_handover", kind: "work", handler: { type: "agent", id: "agent-trade_handover" }, instructions: "", requires: [] },
          ],
        },
      ],
    } as unknown as IssueWorkflow;

    const wf2: IssueWorkflow = {
      id: "wf-2",
      name: "Old Trade Desk · 9f8e7d",
      template_ref: "kv_trade@9f8e7d",
      flow_auto: false,
      open_flow_requests: 0,
      steps: [
        {
          status_key: "collect",
          tasks: [{ key: "docs", title: "trade_docs_reader", kind: "work", handler: { type: "agent", id: "agent-trade_docs_reader" }, instructions: "", requires: [] }],
        },
      ],
    } as unknown as IssueWorkflow;

    mocks.data = {
      active_workflow_id: "wf-1",
      workflows: [wf1, wf2],
      total: 2,
    };
    mocks.isPending = false;
    mocks.isError = false;

    renderWithI18n(<SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={true} />);

    expect(screen.getByText("Import & Export Desk · a1b2c3")).toBeInTheDocument();
    expect(screen.getByText("Old Trade Desk · 9f8e7d")).toBeInTheDocument();
    expect(screen.getByText(/^active$|^đang dùng$/i)).toBeInTheDocument();
    expect(screen.getByText("kv_trade@a1b2c3")).toBeInTheDocument();
  });

  it("offers the auto toggle only on a workflow that runs a flow, never on an ordinary one", () => {
    const flowWf = {
      id: "wf-flow",
      name: "Flow row",
      flow_config: { version: 1, switches: [] },
      flow_auto: false,
      steps: [],
    } as unknown as IssueWorkflow;
    const plainWf = { id: "wf-plain", name: "Plain row", flow_config: null, steps: [] } as unknown as IssueWorkflow;
    mocks.data = { active_workflow_id: "wf-flow", workflows: [flowWf, plainWf], total: 2 };
    mocks.isPending = false;
    mocks.isError = false;

    renderWithI18n(<SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={true} />);

    expect(screen.getAllByRole("button", { name: /auto/i })).toHaveLength(1);
  });

  it("admin controls (set active, toggle auto) appear only when canManage is true", () => {
    const wf: IssueWorkflow = {
      id: "wf-2",
      name: "Alternative Flow",
      template_ref: "kv_trade@alt",
      flow_auto: false,
      open_flow_requests: 0,
      steps: [{ status_key: "todo", tasks: [] }],
    } as unknown as IssueWorkflow;

    mocks.data = {
      active_workflow_id: "wf-1",
      workflows: [wf],
      total: 1,
    };
    mocks.isPending = false;
    mocks.isError = false;

    const { rerender } = renderWithI18n(
      <SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={false} />
    );
    expect(screen.queryByRole("button", { name: /đặt làm đang dùng|set active/i })).not.toBeInTheDocument();

    rerender(<SquadWorkflowsSection wsId="ws-1" squad={baseSquad} canManage={true} />);
    expect(screen.getByRole("button", { name: /đặt làm đang dùng|set active/i })).toBeInTheDocument();
  });
});
