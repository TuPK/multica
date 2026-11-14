import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { setApiInstance } from "@multica/core/api";
import { ApiError, type ApiClient } from "@multica/core/api/client";
import { EMPTY_FLOW_CAPABILITIES_RESPONSE } from "@multica/core/api/schemas";
import type { FlowCapabilitiesResponse, FlowDraft } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { FlowDraftEditorPage } from "./flow-draft-editor-page";

// The page runs on the real hooks of @multica/core/flow-drafts over a fake API client, so what these tests
// see is the calls the page makes: which squad and draft, which revision, which document.

const mocks = vi.hoisted(() => ({
  navReplace: vi.fn(),
  navPush: vi.fn(),
  role: "owner" as "owner" | "admin" | "member",
  download: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    squads: () => "/acme/squads",
    squadDetail: (id: string) => `/acme/squads/${id}`,
    issueDetail: (id: string) => `/acme/issues/${id}`,
  }),
}));
vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({ userId: "u-1", role: mocks.role, member: null, isLoading: false }),
}));
vi.mock("../../navigation", () => ({
  useNavigation: () => ({ push: mocks.navPush, replace: mocks.navReplace, pathname: "/acme/squads/squad-1/workflows/draft-1" }),
  AppLink: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));
vi.mock("./download-json", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./download-json")>()),
  downloadJson: mocks.download,
}));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: mocks.toastSuccess } }));

const fakeApi = {
  getFlowCapabilities: vi.fn(),
  getFlowDraft: vi.fn(),
  updateFlowDraft: vi.fn(),
  createFlowDraft: vi.fn(),
  validateFlowDraft: vi.fn(),
  simulateFlowDraft: vi.fn(),
  publishFlowDraft: vi.fn(),
  exportFlowDraft: vi.fn(),
  setSquadActiveWorkflow: vi.fn(),
  listSquadMembers: vi.fn(),
  listAgents: vi.fn(),
  listMembers: vi.fn(),
  listAttachments: vi.fn(),
  importFlowProposal: vi.fn(),
  getFlowInsights: vi.fn(),
  listSquadSkills: vi.fn(),
  getSkill: vi.fn(),
};

function capabilities(over: Partial<FlowCapabilitiesResponse> = {}, features: Partial<FlowCapabilitiesResponse["features"]> = {}): FlowCapabilitiesResponse {
  return {
    ...EMPTY_FLOW_CAPABILITIES_RESPONSE,
    ...over,
    features: { ...EMPTY_FLOW_CAPABILITIES_RESPONSE.features, authoring: true, graph_v2: true, ...features },
  };
}

function renderPage(ui: React.ReactElement = <FlowDraftEditorPage squadId="squad-1" draftId="draft-1" />) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrap = (node: React.ReactElement) => <QueryClientProvider client={client}>{node}</QueryClientProvider>;
  const result = renderWithI18n(wrap(ui));
  return { ...result, rerender: (next: React.ReactElement) => result.rerender(wrap(next)) };
}

function apiError(status: number, body: Record<string, unknown> = {}, message = "failed") {
  return new ApiError(message, status, "", body);
}

describe("FlowDraftEditorPage", () => {
  const sampleDraft: FlowDraft = {
    id: "draft-1",
    squad_id: "squad-1",
    name: "Customer Onboarding",
    lineage_id: null,
    base_workflow_id: null,
    revision: 1,
    origin: "manual",
    source_ref: null,
    doc: {
      version: 2,
      start: "verify",
      roles: [
        { key: "worker", label: "Worker", kind: "agent", pool: 1 },
        { key: "reviewer", label: "Reviewer", kind: "agent", pool: 1 },
      ],
      vars: [
        { key: "retry_count", type: "int", min: 0, max: 5, default: 0 },
        { key: "unbounded_int", type: "int", default: 0 },
      ],
      verdicts: [
        {
          key: "review",
          verdicts: [
            { key: "pass", class: "pass", label: "Pass" },
            { key: "fail", class: "fail", label: "Fail" },
          ],
        },
      ],
      loops: [
        { key: "fix_loop", header: "verify", max: 3, reset: "on_entry" },
      ],
      terminals: [
        { key: "done", status: "done", leaves_workspace: false },
      ],
      steps: [
        {
          key: "verify",
          title: "Verify Documents",
          kind: "tasks",
          tasks: [
            { key: "verify_doc", title: "Check ID", kind: "work", role: "worker" },
          ],
          edges: [
            { id: "e1", label: "Next", to: "review_step", trigger: "auto" },
          ],
        },
        {
          key: "review_step",
          title: "Legal Review",
          kind: "tasks",
          tasks: [
            { key: "legal_task", title: "Legal review", kind: "gate", role: "reviewer" },
          ],
          edges: [
            { id: "e2", label: "Back", to: "verify", loop: "fix_loop", trigger: "auto" },
            { id: "e3", label: "Done", to: "done", trigger: "auto" },
          ],
        },
      ],
    },
    created_by: "u-1",
    updated_by: "u-1",
    created_at: "2026-11-13T00:00:00Z",
    updated_at: "2026-11-13T00:00:00Z",
  };


  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    mocks.role = "owner";
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.getFlowCapabilities.mockResolvedValue(capabilities());
    fakeApi.getFlowDraft.mockResolvedValue(sampleDraft);
    fakeApi.validateFlowDraft.mockResolvedValue({ ok: true, revision: 1, issues: [], analysis: null, fact_catalog: {} });
    fakeApi.listSquadMembers.mockResolvedValue([
      { id: "m1", squad_id: "squad-1", member_type: "agent", member_id: "agent-lead", role: "leader", created_at: "" },
      { id: "m2", squad_id: "squad-1", member_type: "agent", member_id: "agent-1", role: "", created_at: "" },
    ]);
    fakeApi.listAgents.mockResolvedValue([
      { id: "agent-lead", name: "Lead agent" },
      { id: "agent-1", name: "Writer agent" },
    ]);
    fakeApi.listMembers.mockResolvedValue([{ id: "mem-1", user_id: "user-1", name: "Ana", email: "ana@example.test", role: "member" }]);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  const loaded = async () => {
    const view = renderPage();
    await screen.findByDisplayValue("Customer Onboarding");
    return view;
  };

  it("reads the draft of the squad it is given, with its revision", async () => {
    await loaded();
    expect(fakeApi.getFlowDraft).toHaveBeenCalledWith("squad-1", "draft-1");
  });

  it("load error shows Retry and clicking it reads the draft again", async () => {
    fakeApi.getFlowDraft.mockRejectedValueOnce(apiError(500, {}, "Database error"));
    renderPage();

    expect(await screen.findByText("Could not load the workflow")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByDisplayValue("Customer Onboarding")).toBeInTheDocument();
    expect(fakeApi.getFlowDraft).toHaveBeenCalledTimes(2);
  });

  it("404 returns to the squad page", async () => {
    fakeApi.getFlowDraft.mockRejectedValue(apiError(404, {}, "Not found"));
    renderPage();

    await waitFor(() => expect(mocks.navReplace).toHaveBeenCalledWith("/acme/squads/squad-1"));
  });

  it("an answer the client could not read is not edited: it would be saved over the real draft", async () => {
    // The client falls back to an empty draft without an id for an answer it cannot read.
    fakeApi.getFlowDraft.mockResolvedValue({ ...sampleDraft, id: "" });
    renderPage();

    expect(await screen.findByText("Could not load the workflow")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Customer Onboarding")).toBeNull();
    expect(fakeApi.updateFlowDraft).not.toHaveBeenCalled();
  });

  it("a disabled flag hides the page", async () => {
    fakeApi.getFlowCapabilities.mockResolvedValue(capabilities({}, { authoring: false }));
    renderPage();

    expect(await screen.findByText("Workflow authoring is off")).toBeInTheDocument();
  });

  it("a kind of step the server lists and this build cannot draw shows the unsupported badge", async () => {
    fakeApi.getFlowCapabilities.mockResolvedValue(capabilities({ step_kinds: ["tasks", "lead", "manual", "subflow"] }));
    await loaded();

    expect(await screen.findByText("Not supported by this app version")).toBeInTheDocument();
  });

  it("autosave after 1.5 seconds sends the revision and the document of the draft", async () => {
    await loaded();
    vi.useFakeTimers();
    fakeApi.updateFlowDraft.mockResolvedValue({ ...sampleDraft, name: "Updated Onboarding", revision: 2 });

    fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "Updated Onboarding" } });
    expect(fakeApi.updateFlowDraft).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(fakeApi.updateFlowDraft).toHaveBeenCalledWith("squad-1", "draft-1", {
      revision: 1,
      name: "Updated Onboarding",
      doc: sampleDraft.doc,
    });
  });

  it("an edit made while a save is in flight stays unsaved and is sent by the next save", async () => {
    await loaded();
    vi.useFakeTimers();
    let finish!: (draft: FlowDraft) => void;
    fakeApi.updateFlowDraft.mockImplementationOnce(() => new Promise<FlowDraft>((resolve) => (finish = resolve)));
    fakeApi.updateFlowDraft.mockResolvedValueOnce({ ...sampleDraft, name: "Second", revision: 3 });

    fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "First" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    // The first save is on its way; the person keeps typing.
    fireEvent.change(screen.getByDisplayValue("First"), { target: { value: "Second" } });
    await act(async () => {
      finish({ ...sampleDraft, name: "First", revision: 2 });
      await vi.advanceTimersByTimeAsync(1600);
    });

    expect(fakeApi.updateFlowDraft).toHaveBeenCalledTimes(2);
    expect(fakeApi.updateFlowDraft.mock.calls[1]![2]).toMatchObject({ revision: 2, name: "Second" });
  });

  it("a conflict shows Reload and Keep mine as a copy, and Reload takes the draft the server has", async () => {
    await loaded();
    vi.useFakeTimers();
    fakeApi.updateFlowDraft.mockRejectedValueOnce(apiError(409, { code: "flow_draft_conflict", revision: 3 }));

    fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "Conflicting change" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1600);
    });

    expect(screen.getByText("Someone saved this draft first")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep mine as a copy" })).toBeInTheDocument();

    vi.useRealTimers();
    fakeApi.getFlowDraft.mockResolvedValue({ ...sampleDraft, name: "Saved by someone else", revision: 3 });
    fireEvent.click(screen.getByRole("button", { name: "Reload" }));

    expect(await screen.findByDisplayValue("Saved by someone else")).toBeInTheDocument();
    expect(screen.queryByText("Someone saved this draft first")).toBeNull();
  });

  it("going offline keeps the local buffer and displays Unsaved changes", async () => {
    await loaded();
    vi.useFakeTimers();
    fakeApi.updateFlowDraft.mockRejectedValueOnce(new Error("network error"));

    fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "Offline change" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });

    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Offline change")).toBeInTheDocument();
  });

  it("leaving with unsaved changes prompts before unload", async () => {
    await loaded();
    fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "Dirty name" } });

    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
  });

  it("Check validates the document in the buffer, not only the saved one", async () => {
    await loaded();
    fakeApi.validateFlowDraft.mockResolvedValue({
      ok: false,
      revision: 1,
      issues: [{ code: "role_unused", severity: "error", path: "roles[worker]", message: "role is used by no task" }],
      analysis: null,
      fact_catalog: {},
    });

    fireEvent.click(screen.getByRole("button", { name: "Check" }));

    await waitFor(() => expect(fakeApi.validateFlowDraft).toHaveBeenCalledWith("squad-1", "draft-1", { doc: sampleDraft.doc }));
    expect(await screen.findByText("role is used by no task")).toBeInTheDocument();
  });

  it("Export saves first, then downloads the manifest the server gives", async () => {
    await loaded();
    const manifest = { format: "multica.flow-manifest/v1", name: "Customer Onboarding", doc: sampleDraft.doc };
    fakeApi.exportFlowDraft.mockResolvedValue(manifest);
    fakeApi.updateFlowDraft.mockResolvedValue({ ...sampleDraft, name: "Customer Onboarding v2", revision: 2 });

    fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "Customer Onboarding v2" } });
    fireEvent.click(screen.getByRole("button", { name: "Export" }));

    await waitFor(() => expect(mocks.download).toHaveBeenCalled());
    expect(fakeApi.updateFlowDraft).toHaveBeenCalledTimes(1);
    expect(fakeApi.exportFlowDraft).toHaveBeenCalledWith("squad-1", "draft-1");
    expect(mocks.download).toHaveBeenCalledWith("Customer_Onboarding_v2.manifest.json", manifest);
  });

  describe("publishing", () => {
    const openPublish = async () => {
      await loaded();
      fireEvent.click(screen.getByRole("button", { name: "Publish" }));
      return screen.findByRole("dialog");
    };
    const publishedRow = { id: "wf-9", name: "Customer Onboarding · v1", version: 1 };

    it("sends the revision the server holds after saving the buffer, not the one it loaded", async () => {
      fakeApi.updateFlowDraft.mockResolvedValue({ ...sampleDraft, name: "Customer Onboarding v2", revision: 4 });
      fakeApi.publishFlowDraft.mockResolvedValue(publishedRow);
      await loaded();
      fireEvent.change(screen.getByDisplayValue("Customer Onboarding"), { target: { value: "Customer Onboarding v2" } });
      fireEvent.click(screen.getByRole("button", { name: "Publish" }));
      const dialog = await screen.findByRole("dialog");
      await waitFor(() => expect(within(dialog).getByRole("button", { name: "Publish" })).toBeEnabled());

      fireEvent.click(within(dialog).getByRole("button", { name: "Publish" }));

      await waitFor(() => expect(fakeApi.publishFlowDraft).toHaveBeenCalled());
      expect(fakeApi.publishFlowDraft).toHaveBeenCalledWith("squad-1", "draft-1", { revision: 4, ack: [] });
      expect(fakeApi.setSquadActiveWorkflow).not.toHaveBeenCalled();
      await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/squad-1"));
    });

    it("asks the server what needs acknowledging and sends exactly those", async () => {
      fakeApi.validateFlowDraft.mockResolvedValue({
        ok: true,
        revision: 1,
        issues: [{ code: "no_gate", severity: "warning", message: "no task is a gate", ack: "no_gate" }],
        analysis: null,
        fact_catalog: {},
      });
      fakeApi.publishFlowDraft.mockResolvedValue(publishedRow);
      const dialog = await openPublish();

      const publish = await within(dialog).findByRole("button", { name: "Publish" });
      expect(publish).toBeDisabled();
      fireEvent.click(within(dialog).getByRole("checkbox", { name: /no task is checked by a gate/i }));
      expect(publish).toBeEnabled();
      fireEvent.click(publish);

      await waitFor(() => expect(fakeApi.publishFlowDraft).toHaveBeenCalledWith("squad-1", "draft-1", { revision: 1, ack: ["no_gate"] }));
    });

    it("only an admin can give the acknowledgement that lets work leave unseen", async () => {
      mocks.role = "member";
      fakeApi.validateFlowDraft.mockResolvedValue({
        ok: true,
        revision: 1,
        issues: [{ code: "release_without_human", severity: "warning", message: "leaves unseen", ack: "unattended_release" }],
        analysis: null,
        fact_catalog: {},
      });
      const dialog = await openPublish();

      const box = await within(dialog).findByRole("checkbox", { name: /finish without a person's review/i });
      expect(box).toBeDisabled();
      expect(within(dialog).getByRole("button", { name: "Publish" })).toBeDisabled();
    });

    it("blocks on errors the check found", async () => {
      fakeApi.validateFlowDraft.mockResolvedValue({
        ok: false,
        revision: 1,
        issues: [{ code: "role_unbound", severity: "error", message: "role has nobody bound" }],
        analysis: null,
        fact_catalog: {},
      });
      const dialog = await openPublish();

      expect(await within(dialog).findByText("role has nobody bound")).toBeInTheDocument();
      expect(within(dialog).getByRole("button", { name: "Publish" })).toBeDisabled();
    });

    it("makes the version active through the squad's own route, as the server leaves that to the client", async () => {
      fakeApi.publishFlowDraft.mockResolvedValue(publishedRow);
      fakeApi.setSquadActiveWorkflow.mockResolvedValue({});
      const dialog = await openPublish();
      await waitFor(() => expect(within(dialog).getByRole("button", { name: "Publish" })).toBeEnabled());

      fireEvent.click(within(dialog).getByRole("checkbox", { name: "Make this the active workflow" }));
      fireEvent.click(within(dialog).getByRole("button", { name: "Publish" }));

      await waitFor(() => expect(fakeApi.setSquadActiveWorkflow).toHaveBeenCalledWith("squad-1", { workflow_id: "wf-9" }));
      expect(fakeApi.publishFlowDraft.mock.calls[0]![2]).not.toHaveProperty("make_active");
    });

    it("a version that could not be made active still counts as published, and says so", async () => {
      fakeApi.publishFlowDraft.mockResolvedValue(publishedRow);
      fakeApi.setSquadActiveWorkflow.mockRejectedValue(apiError(422, {}, "needs a mapping"));
      const dialog = await openPublish();
      await waitFor(() => expect(within(dialog).getByRole("button", { name: "Publish" })).toBeEnabled());

      fireEvent.click(within(dialog).getByRole("checkbox", { name: "Make this the active workflow" }));
      fireEvent.click(within(dialog).getByRole("button", { name: "Publish" }));

      await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("needs a mapping"));
      await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/squad-1"));
    });

    it("a stale draft offers a new draft from the latest version", async () => {
      fakeApi.publishFlowDraft.mockRejectedValue(apiError(409, { code: "flow_draft_stale", latest_workflow_id: "wf-latest" }, "stale"));
      fakeApi.createFlowDraft.mockResolvedValue({ ...sampleDraft, id: "draft-2" });
      const dialog = await openPublish();
      await waitFor(() => expect(within(dialog).getByRole("button", { name: "Publish" })).toBeEnabled());

      fireEvent.click(within(dialog).getByRole("button", { name: "Publish" }));
      fireEvent.click(await within(dialog).findByRole("button", { name: "Edit the latest version" }));

      await waitFor(() =>
        expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("squad-1", { from: { kind: "workflow", workflow_id: "wf-latest" } }),
      );
      await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/squad-1/workflows/draft-2"));
    });

    it("a server that wants an acknowledgement the check did not name shows it", async () => {
      fakeApi.publishFlowDraft.mockRejectedValue(apiError(422, { code: "flow_ack_required", required: ["no_gate"] }, "needs an ack"));
      const dialog = await openPublish();
      await waitFor(() => expect(within(dialog).getByRole("button", { name: "Publish" })).toBeEnabled());

      fireEvent.click(within(dialog).getByRole("button", { name: "Publish" }));

      expect(await within(dialog).findByRole("checkbox", { name: /no task is checked by a gate/i })).toBeInTheDocument();
    });
  });

  it("Simulate sends the document in the buffer, the variables and the results the person chose", async () => {
    fakeApi.simulateFlowDraft.mockResolvedValue({
      hops: [{ n: 0, step: "verify", activation: 1, edge: "e1", to: "review_step" }],
      end: { kind: "terminal", code: "done" },
      truncated: false,
    });
    await loaded();
    fireEvent.click(screen.getByRole("button", { name: "Simulate" }));
    const dialog = await screen.findByRole("dialog");

    fireEvent.change(within(dialog).getByRole("combobox", { name: /review_step\s*\/\s*legal review/i }), { target: { value: "rejected" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Run" }));

    await waitFor(() => expect(fakeApi.simulateFlowDraft).toHaveBeenCalled());
    const [squadId, draftId, request] = fakeApi.simulateFlowDraft.mock.calls[0]!;
    expect([squadId, draftId]).toEqual(["squad-1", "draft-1"]);
    expect(request.doc).toEqual(sampleDraft.doc);
    expect(request.max_hops).toBe(60);
    expect(request.vars).toEqual({});
    expect(request.script.review_step).toHaveLength(21);
    expect(request.script.review_step[0]).toEqual({ legal_task: "rejected" });
    expect(await within(dialog).findByText("terminal: done")).toBeInTheDocument();
    expect(request).not.toHaveProperty("switches");
    expect(request).not.toHaveProperty("path_choices");
  });

  it("a role in use cannot be deleted, but an unused role can be deleted", async () => {
    await loaded();

    // Switch to Roles tab
    fireEvent.click(screen.getByRole("button", { name: "Roles" }));

    // Role "worker" is in use by verify_doc -> delete button should NOT exist
    expect(screen.queryByRole("button", { name: "Delete worker" })).toBeNull();
  });

  it("adding a loop with max from a variable only lists int variables that have a max", async () => {
    await loaded();

    // Switch to Loops tab
    fireEvent.click(screen.getByRole("button", { name: "Loops" }));

    // Switch max mode to "From variable"
    const selects = screen.getAllByRole("combobox");
    const modeSelect = selects.find((s) => within(s).queryByText("Fixed count"));
    if (modeSelect) {
      fireEvent.change(modeSelect, { target: { value: "var" } });
    }

    const varSelect = screen.getByRole("combobox", { name: "Eligible integer variable" });
    // retry_count (max: 5) is eligible; unbounded_int has no max so must not be in list
    expect(within(varSelect).getByText("retry_count (max: 5)")).toBeInTheDocument();
    expect(within(varSelect).queryByText(/unbounded_int/)).toBeNull();
  });

  it("drag-reordering or moving edges changes the saved order", async () => {
    await loaded();

    // Step "Legal Review" has 2 edges: #1 Back, #2 Done
    fireEvent.click(screen.getByText("Legal Review"));

    const moveDownBtn = screen.getByRole("button", { name: "Move edge 1 down" });
    fireEvent.click(moveDownBtn);

    // Now #1 is Done and #2 is Back
    const inputs = screen.getAllByPlaceholderText("Edge label");
    expect(inputs[0]).toHaveValue("Done");
    expect(inputs[1]).toHaveValue("Back");
  });

  // A role is given to the agents of the squad besides its lead, and to people of the workspace (spec 11.2).
  it("gives an agent role to an agent of the squad, never to its lead, and saves the binding", async () => {
    await loaded();
    fireEvent.click(screen.getByRole("button", { name: "Roles" }));
    // Both roles of the draft list the agents of the squad; the first row is the worker.
    const writer = (await screen.findAllByRole("checkbox", { name: "Writer agent" }))[0]!;
    expect(screen.queryByRole("checkbox", { name: "Lead agent" })).toBeNull();
    vi.useFakeTimers();
    fakeApi.updateFlowDraft.mockResolvedValue({ ...sampleDraft, revision: 2 });

    fireEvent.click(writer);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    const saved = fakeApi.updateFlowDraft.mock.calls[0]![2].doc;
    expect(saved.roles.find((r: { key: string }) => r.key === "worker").bind).toEqual({ agent_ids: ["agent-1"] });
  });

  describe("adding a step", () => {
    it("offers a blank step and the five templates", async () => {
      await loaded();
      fireEvent.click(screen.getByRole("button", { name: "Add step" }));
      const names = (await screen.findAllByRole("menuitem")).map((i) => i.textContent);
      expect(names).toEqual(["Blank step", "Work", "Gate", "Quorum review", "Human approval", "Lead decision"]);
    });

    it("a Gate template adds a gate with its standard edges, selects it, and the save carries it", async () => {
      await loaded();
      fireEvent.click(screen.getByRole("button", { name: "Add step" }));
      const gateItem = await screen.findByRole("menuitem", { name: "Gate" });
      vi.useFakeTimers();
      fakeApi.updateFlowDraft.mockResolvedValue({ ...sampleDraft, revision: 2 });
      fireEvent.click(gateItem);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500);
      });

      const saved = fakeApi.updateFlowDraft.mock.calls[0]![2].doc;
      expect(saved.steps).toHaveLength(3);
      const gate = saved.steps[2];
      expect(gate).toMatchObject({ key: "gate_1", title: "Gate", kind: "tasks" });
      // The draft already has a loop, so a rejected gate can go back along it.
      expect(gate.edges.map((e: { label: string }) => e.label)).toEqual(["Blocked", "Passed", "Send back for rework", "Stopped"]);
      expect(gate.tasks[0]).toMatchObject({ kind: "gate", role: "reviewer" });
      // The step shown in the inspector is the new one.
      expect(screen.getByDisplayValue("Gate")).toBeInTheDocument();
      // The steps that were there are untouched.
      expect(saved.steps.slice(0, 2)).toEqual(sampleDraft.doc.steps);
    });

    it("a blank step is still empty", async () => {
      await loaded();
      fireEvent.click(screen.getByRole("button", { name: "Add step" }));
      const blankItem = await screen.findByRole("menuitem", { name: "Blank step" });
      vi.useFakeTimers();
      fakeApi.updateFlowDraft.mockResolvedValue({ ...sampleDraft, revision: 2 });
      fireEvent.click(blankItem);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500);
      });

      const saved = fakeApi.updateFlowDraft.mock.calls[0]![2].doc;
      expect(saved.steps[2]).toMatchObject({ key: "step_3", kind: "tasks", tasks: [], edges: [] });
    });
  });

  it("the release warning shows and only an admin can tick the acknowledgement", async () => {
    mocks.role = "member";
    fakeApi.getFlowDraft.mockResolvedValue({
      ...sampleDraft,
      doc: { ...sampleDraft.doc, terminals: [{ key: "external_out", status: "closed", leaves_workspace: true }] },
    });
    const { rerender } = renderPage();
    await screen.findByDisplayValue("Customer Onboarding");

    fireEvent.click(screen.getByRole("button", { name: "Terminals" }));

    expect(screen.getByText("This workflow can leave the workspace without a person's review")).toBeInTheDocument();
    const checkbox = screen.getByRole("checkbox", { name: /I understand this workflow can finish without a person's review/i });
    expect(checkbox).toBeDisabled();

    mocks.role = "admin";
    rerender(<FlowDraftEditorPage squadId="squad-1" draftId="draft-1" />);
    expect(screen.getByRole("checkbox", { name: /I understand this workflow can finish without a person's review/i })).not.toBeDisabled();
  });
});

// The R3 surfaces of the editor: the proposal banner, the numbers of the version the draft comes from, and the reference.
describe("FlowDraftEditorPage, proposals, insights and reference", () => {
  const draft = (over: Partial<FlowDraft> = {}): FlowDraft => ({
    id: "draft-1",
    squad_id: "squad-1",
    name: "Proposed flow",
    lineage_id: null,
    base_workflow_id: null,
    revision: 1,
    origin: "manual",
    source_ref: null,
    doc: {
      version: 2,
      start: "plan",
      terminals: [{ key: "done", status: "done" }],
      steps: [{ key: "plan", title: "Plan", kind: "lead", edges: [{ id: "plan_ok", label: "Planned", to: "done" }] }],
    },
    created_by: "u-1",
    updated_by: "u-1",
    created_at: "2026-11-13T00:00:00Z",
    updated_at: "2026-11-13T00:00:00Z",
    ...over,
  });
  const report = {
    requests: { total: 7, open: 0, by_end: {} },
    steps: [{ key: "plan", entered: 7, median_minutes: 3, p90_minutes: 9 }],
    edges: [{ step: "plan", edge: "plan_ok", taken: 7 }],
    loops: [],
    stalls: [],
    manual_closes: 0,
    truncated: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.role = "owner";
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.getFlowCapabilities.mockResolvedValue(capabilities({}, { insights: true, proposals: true }));
    fakeApi.getFlowDraft.mockResolvedValue(draft());
    fakeApi.listSquadMembers.mockResolvedValue([]);
    fakeApi.listAgents.mockResolvedValue([{ id: "ag-1", name: "Designer", archived_at: null }]);
    fakeApi.listMembers.mockResolvedValue([]);
    fakeApi.listAttachments.mockResolvedValue([]);
    fakeApi.getFlowInsights.mockResolvedValue(report);
  });

  it("waits for the agent on a proposal, then offers Import proposal and shows the draft the server answers with", async () => {
    fakeApi.getFlowDraft.mockResolvedValue(draft({ origin: "proposal", source_ref: { issue_id: "iss-1", agent_id: "ag-1" } }));
    renderPage();
    expect(await screen.findByText("Waiting for the agent's proposal")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Import proposal" })).toBeNull();
  });

  it("imports the file of the agent and shows the document it brings", async () => {
    fakeApi.getFlowDraft.mockResolvedValue(draft({ origin: "proposal", source_ref: { issue_id: "iss-1", agent_id: "ag-1" } }));
    fakeApi.listAttachments.mockResolvedValue([{ id: "a-1", filename: "flow.json", uploader_type: "agent", uploader_id: "ag-1", created_at: "2026-11-14T10:00:00Z" }]);
    fakeApi.importFlowProposal.mockResolvedValue(draft({ origin: "proposal", name: "From the agent", revision: 2, updated_at: "2026-11-14T11:00:00Z", source_ref: { issue_id: "iss-1", agent_id: "ag-1" } }));
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Import proposal" }));

    await waitFor(() => expect(fakeApi.importFlowProposal).toHaveBeenCalledWith("squad-1", "draft-1", { revision: 1 }));
    expect(await screen.findByDisplayValue("From the agent")).toBeInTheDocument();
    expect(await screen.findByText("Proposed by Designer")).toBeInTheDocument();
  });

  it("has no banner on a draft that nobody proposed", async () => {
    renderPage();
    await screen.findByDisplayValue("Proposed flow");
    expect(screen.queryByText("Waiting for the agent's proposal")).toBeNull();
    expect(fakeApi.listAttachments).not.toHaveBeenCalled();
  });

  it("shows the numbers of the version the draft comes from on its graph, and reads again with trial runs", async () => {
    fakeApi.getFlowDraft.mockResolvedValue(draft({ base_workflow_id: "wf-1" }));
    renderPage();
    await screen.findByDisplayValue("Proposed flow");
    fireEvent.click(screen.getByRole("button", { name: "Graph" }));

    expect((await screen.findAllByText("Entered 7 times")).length).toBeGreaterThan(0);
    expect(fakeApi.getFlowInsights).toHaveBeenCalledWith("wf-1", { includeTrial: false });
    expect(screen.getByText("Numbers from 7 requests")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: "Include trial runs" }));
    await waitFor(() => expect(fakeApi.getFlowInsights).toHaveBeenCalledWith("wf-1", { includeTrial: true }));
  });

  it("shows no numbers for a draft with no version behind it, or one nobody has run", async () => {
    renderPage();
    await screen.findByDisplayValue("Proposed flow");
    fireEvent.click(screen.getByRole("button", { name: "Graph" }));
    expect(fakeApi.getFlowInsights).not.toHaveBeenCalled();
    expect(screen.queryByText(/Entered/)).toBeNull();

    cleanup();
    fakeApi.getFlowDraft.mockResolvedValue(draft({ base_workflow_id: "wf-1" }));
    fakeApi.getFlowInsights.mockResolvedValue({ ...report, requests: { total: 0, open: 0, by_end: {} }, steps: [], edges: [] });
    renderPage();
    await screen.findByDisplayValue("Proposed flow");
    fireEvent.click(screen.getByRole("button", { name: "Graph" }));
    await waitFor(() => expect(fakeApi.getFlowInsights).toHaveBeenCalled());
    expect(screen.queryByText(/Entered|Numbers from/)).toBeNull();
  });

  it("asks for no numbers when the server says the feature is off", async () => {
    fakeApi.getFlowCapabilities.mockResolvedValue(capabilities({}, { insights: false }));
    fakeApi.getFlowDraft.mockResolvedValue(draft({ base_workflow_id: "wf-1" }));
    renderPage();
    await screen.findByDisplayValue("Proposed flow");
    fireEvent.click(screen.getByRole("button", { name: "Graph" }));
    await waitFor(() => expect(fakeApi.getFlowCapabilities).toHaveBeenCalled());
    expect(fakeApi.getFlowInsights).not.toHaveBeenCalled();
    expect(screen.queryByText(/Entered/)).toBeNull();
  });

  it("offers the reference next to the editor, folded", async () => {
    renderPage();
    await screen.findByDisplayValue("Proposed flow");
    expect(screen.getByRole("button", { name: "Show reference" })).toBeInTheDocument();
    expect(fakeApi.listSquadSkills).not.toHaveBeenCalled();
  });
});
