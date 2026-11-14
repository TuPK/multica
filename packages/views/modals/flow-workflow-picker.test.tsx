import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { EMPTY_FLOW_CAPABILITIES_RESPONSE } from "@multica/core/api/schemas";
import type { IssueWorkflow } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { countFlowLineages, FlowWorkflowPicker } from "./flow-workflow-picker";

// The picker runs on the real query over a fake API client.

const fakeApi = { listSquadWorkflows: vi.fn(), getFlowCapabilities: vi.fn() };

function capabilities(authoring: boolean) {
  fakeApi.getFlowCapabilities.mockResolvedValue({
    ...EMPTY_FLOW_CAPABILITIES_RESPONSE,
    features: { ...EMPTY_FLOW_CAPABILITIES_RESPONSE.features, authoring },
  });
}

const flow = (id: string, name: string, lineage: string | null, version: number | null = null): IssueWorkflow =>
  ({ id, name, steps: [], flow_config: { version: 2, switches: [] }, lineage_id: lineage, version }) as unknown as IssueWorkflow;
const plain = (id: string): IssueWorkflow => ({ id, name: "Project workflow", steps: [], flow_config: null }) as unknown as IssueWorkflow;

function serve(workflows: IssueWorkflow[], active: string | null) {
  fakeApi.listSquadWorkflows.mockResolvedValue({ active_workflow_id: active, workflows, total: workflows.length });
}

function renderPicker(value: string | undefined, onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = renderWithI18n(
    <QueryClientProvider client={client}>
      <FlowWorkflowPicker wsId="ws-1" squadId="sq-1" value={value} onChange={onChange} />
    </QueryClientProvider>,
  );
  return { ...view, onChange };
}

describe("FlowWorkflowPicker", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    capabilities(true);
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("counts a lineage once however many versions it has, and ignores workflows that run on no flow", () => {
    expect(countFlowLineages([flow("a", "A v1", "L1", 1), flow("b", "A v2", "L1", 2), plain("p")])).toBe(1);
    expect(countFlowLineages([flow("a", "A v1", "L1", 1), flow("b", "B v1", "L2", 1)])).toBe(2);
    // Without a lineage a workflow is a flow of its own.
    expect(countFlowLineages([flow("a", "A", null), flow("b", "B", null)])).toBe(2);
  });

  it("shows nothing when the squad has one flow, however many versions", async () => {
    serve([flow("a", "Write v1", "L1", 1), flow("b", "Write v2", "L1", 2), plain("p")], "b");
    renderPicker(undefined);
    await waitFor(() => expect(fakeApi.listSquadWorkflows).toHaveBeenCalledWith("sq-1"));
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("shows the active workflow by default and offers every flow of the squad", async () => {
    serve([flow("a", "Article v1", "L1", 1), flow("b", "Newsletter v1", "L2", 1), plain("p")], "a");
    renderPicker(undefined);
    const trigger = await screen.findByRole("button", { name: "Workflow" });
    expect(trigger).toHaveTextContent("Article v1 (active)");

    fireEvent.click(trigger);
    const options = await screen.findAllByRole("menuitemradio");
    expect(options.map((o) => o.textContent)).toEqual(["Article v1 (active)", "Newsletter v1"]);
    expect(options[0]).toHaveAttribute("aria-checked", "true");
  });

  it("shows nothing when the server has flow authoring off, however many flows the squad has", async () => {
    capabilities(false);
    serve([flow("a", "Article v1", "L1", 1), flow("b", "Newsletter v1", "L2", 1)], "a");
    renderPicker(undefined);
    await waitFor(() => expect(fakeApi.getFlowCapabilities).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByRole("button")).toBeNull());
    // It does not stay hidden by accident: the same squad shows the picker with authoring on.
    cleanup();
    capabilities(true);
    renderPicker(undefined);
    expect(await screen.findByRole("button", { name: "Workflow" })).toBeInTheDocument();
  });

  it("reports the chosen workflow, and undefined when the active one is chosen again", async () => {
    serve([flow("a", "Article v1", "L1", 1), flow("b", "Newsletter v1", "L2", 1)], "a");
    const first = renderPicker(undefined);
    fireEvent.click(await screen.findByRole("button", { name: "Workflow" }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: "Newsletter v1" }));
    expect(first.onChange).toHaveBeenCalledWith("b");
    cleanup();

    const second = renderPicker("b");
    const trigger = await screen.findByRole("button", { name: "Workflow" });
    expect(trigger).toHaveTextContent("Newsletter v1");
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("menuitemradio", { name: "Article v1 (active)" }));
    expect(second.onChange).toHaveBeenCalledWith(undefined);
  });
});
