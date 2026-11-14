import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { EMPTY_FLOW_CAPABILITIES_RESPONSE } from "@multica/core/api/schemas";
import type { IssueWorkflow, Squad, SquadWorkflowsResponse } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { SquadWorkflowsSection } from "./squad-workflows-section";

// The section runs on the real hooks over a fake API client: what the tests check is the calls it makes.

const mocks = vi.hoisted(() => ({ navPush: vi.fn(), download: vi.fn(), toastError: vi.fn() }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("../../navigation", () => ({
  useNavigation: () => ({ push: mocks.navPush, replace: vi.fn(), pathname: "/acme/squads/sq-1" }),
  AppLink: ({ children, href, ...props }: React.ComponentProps<"a">) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    squads: () => "/acme/squads",
    squadDetail: (id: string) => `/acme/squads/${id}`,
    issueDetail: (id: string) => `/acme/issues/${id}`,
  }),
}));
vi.mock("../flow-designer/download-json", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../flow-designer/download-json")>()),
  downloadJson: mocks.download,
}));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: vi.fn() } }));

const fakeApi = {
  listSquadWorkflows: vi.fn(),
  getFlowCapabilities: vi.fn(),
  listFlowDrafts: vi.fn(),
  createFlowDraft: vi.fn(),
  getIssueWorkflowFlowManifest: vi.fn(),
  setSquadActiveWorkflow: vi.fn(),
  setIssueWorkflowFlowAuto: vi.fn(),
  createIssue: vi.fn(),
};

const baseSquad = {
  id: "sq-1",
  name: "Trade Squad",
  template_key: "kv_trade",
  active_workflow_id: "wf-1",
} as unknown as Squad;

const flowRow = (over: Partial<IssueWorkflow>): IssueWorkflow =>
  ({ steps: [], flow_config: { version: 1, switches: [] }, flow_auto: false, ...over }) as unknown as IssueWorkflow;

function serve(workflows: IssueWorkflow[], activeId: string | null = null) {
  fakeApi.listSquadWorkflows.mockResolvedValue({ active_workflow_id: activeId, workflows, total: workflows.length } satisfies SquadWorkflowsResponse);
}

function capabilities(authoring: boolean) {
  fakeApi.getFlowCapabilities.mockResolvedValue({
    ...EMPTY_FLOW_CAPABILITIES_RESPONSE,
    features: { ...EMPTY_FLOW_CAPABILITIES_RESPONSE.features, authoring },
  });
}

function renderSection(canManage: boolean, squad: Squad = baseSquad) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const ui = (manage: boolean) => (
    <QueryClientProvider client={client}>
      <SquadWorkflowsSection wsId="ws-1" squad={squad} canManage={manage} />
    </QueryClientProvider>
  );
  const view = renderWithI18n(ui(canManage));
  return { ...view, rerenderAs: (manage: boolean) => view.rerender(ui(manage)) };
}

describe("SquadWorkflowsSection", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    capabilities(true);
    serve([]);
    fakeApi.listFlowDrafts.mockResolvedValue({ drafts: [], total: 0 });
    fakeApi.createFlowDraft.mockResolvedValue({ id: "draft-new-1" });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders the section title and an empty state without actions for someone who cannot manage", async () => {
    renderSection(false);

    expect(await screen.findByText(/no workflows yet/i)).toBeInTheDocument();
    expect(screen.getByText("Workflows")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^blank$/i })).toBeNull();
    // The drafts of a squad are its managers': nobody else asks for them.
    expect(fakeApi.listFlowDrafts).not.toHaveBeenCalled();
  });

  it("offers a blank draft and an import in the empty state, and no copy of a workflow that is not there", async () => {
    renderSection(true);

    expect(await screen.findByRole("button", { name: /^blank$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /import a file/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /copy a kit việt workflow/i })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /^blank$/i }));
    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("sq-1", { name: "Trade Squad", from: { kind: "blank" } }));
    await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/sq-1/workflows/draft-new-1"));
  });

  it("hides 'New workflow' when the person cannot manage the squad or authoring is off", async () => {
    const view = renderSection(false);
    await screen.findByText(/no workflows yet/i);
    expect(screen.queryByRole("button", { name: /new workflow/i })).toBeNull();

    view.rerenderAs(true);
    expect(await screen.findByRole("button", { name: /new workflow/i })).toBeInTheDocument();

    cleanup();
    capabilities(false);
    renderSection(true);
    await screen.findByText(/no workflows yet/i);
    await waitFor(() => expect(fakeApi.getFlowCapabilities).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByRole("button", { name: /new workflow/i })).toBeNull());
  });

  it("shows the drafts of the squad to its managers, each linking to its editor", async () => {
    fakeApi.listFlowDrafts.mockResolvedValue({ drafts: [{ id: "draft-7", name: "Intake v2", squad_id: "sq-1" }], total: 1 });
    renderSection(true);

    const link = await screen.findByRole("link", { name: /intake v2/i });
    expect(link).toHaveAttribute("href", "/acme/squads/sq-1/workflows/draft-7");
    expect(fakeApi.listFlowDrafts).toHaveBeenCalledWith("sq-1");
  });

  it("renders the error state with a retry that reads the workflows again", async () => {
    fakeApi.listSquadWorkflows.mockRejectedValueOnce(new Error("boom"));
    renderSection(false);

    fireEvent.click(await screen.findByRole("button", { name: /try again/i }));

    await waitFor(() => expect(fakeApi.listSquadWorkflows).toHaveBeenCalledTimes(2));
  });

  it("groups versions of one lineage together with their version pills", async () => {
    serve(
      [
        flowRow({ id: "wf-lineage-1", name: "Desk v1", lineage_id: "lineage-trade-desk", version: 1, published_at: "2026-11-01T00:00:00Z" }),
        flowRow({ id: "wf-lineage-2", name: "Desk v2", lineage_id: "lineage-trade-desk", version: 2, published_at: "2026-11-02T00:00:00Z" }),
      ],
      "wf-lineage-2",
    );
    renderSection(true);

    expect(await screen.findByText("Desk v2")).toBeInTheDocument();
    expect(screen.getByText("Desk v1")).toBeInTheDocument();
    expect(screen.getByText("v1")).toBeInTheDocument();
    expect(screen.getByText("v2")).toBeInTheDocument();
    expect(screen.getByTestId("lineage-group-lineage-trade-desk")).toBeInTheDocument();
  });

  it("copies a published version as a draft: the server names it, and the lineage rides on the workflow id", async () => {
    serve([flowRow({ id: "wf-pub", name: "Published Workflow · v1", version: 1, published_at: "2026-11-01T00:00:00Z" })], "wf-pub");
    renderSection(true);

    fireEvent.click(await screen.findByRole("button", { name: /edit as draft/i }));

    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("sq-1", { from: { kind: "workflow", workflow_id: "wf-pub" } }));
    await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/sq-1/workflows/draft-new-1"));
  });

  it("copies a workflow of schema 1 the same way, a Kit Việt one included", async () => {
    serve([flowRow({ id: "wf-kv", name: "Trade · abc", template_ref: "flow_kv_trade@abc" })], "wf-kv");
    renderSection(true);

    fireEvent.click(await screen.findByRole("button", { name: /edit as draft/i }));

    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("sq-1", { from: { kind: "workflow", workflow_id: "wf-kv" } }));
  });

  it("exports the manifest of a flow workflow the server gives, and downloads it", async () => {
    const manifest = { format: "multica.flow-manifest/v1", name: "Desk", doc: { version: 2 } };
    fakeApi.getIssueWorkflowFlowManifest.mockResolvedValue(manifest);
    serve([flowRow({ id: "wf-pub", name: "Desk · v1", version: 1 })], "wf-pub");
    renderSection(true);

    fireEvent.click(await screen.findByRole("button", { name: /^export$/i }));

    await waitFor(() => expect(mocks.download).toHaveBeenCalledWith("Desk_v1.manifest.json", manifest));
    expect(fakeApi.getIssueWorkflowFlowManifest).toHaveBeenCalledWith("wf-pub");
  });

  it("hides edit and export from someone who cannot manage the squad", async () => {
    serve([flowRow({ id: "wf-pub", name: "Desk · v1", version: 1 })], "wf-pub");
    renderSection(false);

    await screen.findByText("Desk · v1");
    expect(screen.queryByRole("button", { name: /edit as draft/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /^export$/i })).toBeNull();
  });

  it("offers the auto toggle only on a workflow that runs a flow, never on an ordinary one", async () => {
    serve([flowRow({ id: "wf-flow", name: "Flow row" }), { id: "wf-plain", name: "Plain row", flow_config: null, steps: [] } as unknown as IssueWorkflow], "wf-flow");
    renderSection(true);

    await screen.findByText("Flow row");
    expect(screen.getAllByRole("button", { name: /^auto/i })).toHaveLength(1);
  });

  it("sets a version active through the squad's own route", async () => {
    fakeApi.setSquadActiveWorkflow.mockResolvedValue({});
    serve([flowRow({ id: "wf-2", name: "Alternative Flow", template_ref: "kv_trade@alt" })], "wf-1");
    renderSection(true);

    fireEvent.click(await screen.findByRole("button", { name: /set active/i }));

    await waitFor(() => expect(fakeApi.setSquadActiveWorkflow).toHaveBeenCalledWith("sq-1", { workflow_id: "wf-2" }));
  });

  it("shows the admin controls only to someone who can manage the squad", async () => {
    serve([flowRow({ id: "wf-2", name: "Alternative Flow", template_ref: "kv_trade@alt" })], "wf-1");
    const view = renderSection(false);

    await screen.findByText("Alternative Flow");
    expect(screen.queryByRole("button", { name: /set active/i })).toBeNull();

    view.rerenderAs(true);
    expect(await screen.findByRole("button", { name: /set active/i })).toBeInTheDocument();
  });

  describe("trial run", () => {
    const published = (over: Partial<IssueWorkflow> = {}) =>
      flowRow({ id: "wf-v2", name: "Desk · v2", version: 2, published_at: "2026-11-01T00:00:00Z", flow_config: { version: 2, switches: [] }, ...over });

    it("is offered to a manager on a published version of schema 2, whether or not it is the active one", async () => {
      serve([published(), published({ id: "wf-v1", name: "Desk · v1", version: 1 })], "wf-v1");
      renderSection(true);

      await screen.findByText("Desk · v2");
      expect(screen.getAllByRole("button", { name: /trial run/i })).toHaveLength(2);
    });

    it("is not offered for a workflow of schema 1, a version not published, someone who cannot manage, or with authoring off", async () => {
      serve([
        flowRow({ id: "wf-kv", name: "Catalog flow", template_ref: "flow_kv@x" }),
        published({ id: "wf-unpublished", name: "Unpublished", published_at: null }),
      ], "wf-kv");
      renderSection(true);
      await screen.findByText("Catalog flow");
      expect(screen.queryByRole("button", { name: /trial run/i })).toBeNull();
      cleanup();

      serve([published()], "wf-v2");
      renderSection(false);
      await screen.findByText("Desk · v2");
      expect(screen.queryByRole("button", { name: /trial run/i })).toBeNull();
      cleanup();

      capabilities(false);
      renderSection(true);
      await screen.findByText("Desk · v2");
      await waitFor(() => expect(fakeApi.getFlowCapabilities).toHaveBeenCalled());
      await waitFor(() => expect(screen.queryByRole("button", { name: /trial run/i })).toBeNull());
    });

    it("starts a rehearsal of that version on a sample request the user titles, and opens the request", async () => {
      fakeApi.createIssue.mockResolvedValue({ id: "iss-trial-1" });
      serve([published(), published({ id: "wf-v1", name: "Desk · v1", version: 1 })], "wf-v1");
      renderSection(true);
      await screen.findByText("Desk · v2");

      // The first row is the newest version, not the one the squad runs.
      fireEvent.click(screen.getAllByRole("button", { name: /trial run/i })[0]!);
      const dialog = await screen.findByRole("dialog");
      const start = within(dialog).getByRole("button", { name: /^start$/i });
      expect(start).toBeDisabled();

      fireEvent.change(within(dialog).getByLabelText(/title of the sample request/i), { target: { value: "  Check a sample brief  " } });
      fireEvent.click(start);

      await waitFor(() =>
        expect(fakeApi.createIssue).toHaveBeenCalledWith({
          title: "Check a sample brief",
          assignee_type: "squad",
          assignee_id: "sq-1",
          flow_workflow_id: "wf-v2",
          flow_trial: true,
        }),
      );
      await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/issues/iss-trial-1"));
    });

    it("keeps the dialog open and says so when the server refuses", async () => {
      fakeApi.createIssue.mockRejectedValue(new Error("the workflow does not belong to the squad of this request"));
      serve([published()], "wf-v2");
      renderSection(true);
      fireEvent.click(await screen.findByRole("button", { name: /trial run/i }));
      const dialog = await screen.findByRole("dialog");
      fireEvent.change(within(dialog).getByLabelText(/title of the sample request/i), { target: { value: "Sample" } });
      fireEvent.click(within(dialog).getByRole("button", { name: /^start$/i }));

      await waitFor(() => expect(mocks.toastError).toHaveBeenCalled());
      expect(mocks.navPush).not.toHaveBeenCalled();
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });
});
