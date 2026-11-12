// @vitest-environment jsdom

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssueWorkflow, IssueWorkflowTemplate } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { WorkflowDesignerPage } from "./workflow-designer-page";
// Template fixtures in the server's shape: a created status is named, not keyed.
const noHandler = { type: "none" as const };
const TEMPLATES: IssueWorkflowTemplate[] = [
  {
    id: "delivery",
    steps: [
      { status_key: "todo", handler: noHandler, instructions: "", transitions: [{ id: "d-todo", label: "Default", when: null, to_status_key: "in_progress" }] },
      { status_key: "in_progress", handler: noHandler, instructions: "", transitions: [{ id: "d-prog", label: "Default", when: null, to_status_key: "in_review" }] },
      { status_key: "in_review", handler: noHandler, instructions: "", transitions: [{ id: "d-rev", label: "Default", when: null, to_status_key: "qa" }] },
      { new_status: { name: "QA", category: "started" }, handler: noHandler, instructions: "", transitions: [{ id: "d-qa", label: "Default", when: null, to_status_key: "done" }] },
      { status_key: "done", handler: noHandler, instructions: "", transitions: [] },
      { status_key: "cancelled", handler: noHandler, instructions: "", transitions: [] },
    ],
  },
  {
    id: "content",
    steps: [
      { status_key: "todo", handler: noHandler, instructions: "", transitions: [{ id: "c-todo", label: "Default", when: null, to_status_key: "drafting" }] },
      { new_status: { name: "Drafting", category: "started" }, handler: noHandler, instructions: "", transitions: [{ id: "c-draft", label: "Default", when: null, to_status_key: "done" }] },
      { status_key: "done", handler: noHandler, instructions: "", transitions: [] },
      { status_key: "cancelled", handler: noHandler, instructions: "", transitions: [] },
    ],
  },
];

const { apiMock, testWorkflow } = vi.hoisted(() => {
  const testWorkflow = {
    id: "wf-1",
    workspace_id: "ws-1",
    name: "Core Pipeline",
    description: "Development workflow",
    is_default: true,
    owner_id: null,
    steps: [
      {
        status_key: "todo",
        handler: { type: "none" },
        instructions: "",
        transitions: [{ id: "t1", label: "Start", when: null, to_status_key: "in_progress" }],
      },
      {
        status_key: "in_progress",
        handler: { type: "none" },
        instructions: "",
        transitions: [{ id: "t2", label: "Done", when: null, to_status_key: "done" }],
      },
      { status_key: "done", handler: { type: "none" }, instructions: "", transitions: [] },
      { status_key: "cancelled", handler: { type: "none" }, instructions: "", transitions: [] },
    ],
    project_ids: [],
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-15T00:00:00Z",
  } as IssueWorkflow;
  return {
    testWorkflow,
    apiMock: {
      getIssueWorkflow: vi.fn(),
      listIssueWorkflows: vi.fn(),
      listIssueWorkflowUsage: vi.fn(),
      createIssueWorkflow: vi.fn(),
      updateIssueWorkflow: vi.fn(),
      listIssueWorkflowTemplates: vi.fn(),
    },
  };
});

vi.mock("@multica/core/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/api")>();
  return { ...actual, api: apiMock };
});

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/paths", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/paths")>();
  return {
    ...actual,
    useWorkspacePaths: () => actual.paths.workspace("acme"),
  };
});

vi.mock("@multica/core/issue-statuses/hooks", () => {
  // The real hook returns a stable catalog between renders; so does this mock.
  const catalog = {
    statuses: [
      { id: "s-todo", key: "todo", name: "To Do" },
      { id: "s-inp", key: "in_progress", name: "In Progress" },
      { id: "s-rev", key: "in_review", name: "In Review" },
      { id: "s-done", key: "done", name: "Done" },
      { id: "s-can", key: "cancelled", name: "Cancelled" },
    ],
    entryOf: (k: string) => {
      const map: Record<string, { id: string; key: string; name: string }> = {
        todo: { id: "s-todo", key: "todo", name: "To Do" },
        in_progress: { id: "s-inp", key: "in_progress", name: "In Progress" },
        in_review: { id: "s-rev", key: "in_review", name: "In Review" },
        done: { id: "s-done", key: "done", name: "Done" },
        cancelled: { id: "s-can", key: "cancelled", name: "Cancelled" },
      };
      return map[k];
    },
  };
  return { useIssueStatuses: () => catalog };
});

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"], queryFn: () => [] }),
  agentListOptions: () => ({ queryKey: ["agents"], queryFn: () => [] }),
}));

function makeAdapter(overrides: Partial<NavigationAdapter> = {}): NavigationAdapter {
  return {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    prefetch: vi.fn(),
    pathname: "/acme/workflow/wf-1",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (p) => p,
    ...overrides,
  };
}

describe("WorkflowDesignerPage", () => {
  let queryClient: QueryClient;
  let adapter: NavigationAdapter;

  beforeEach(() => {
    apiMock.getIssueWorkflow.mockResolvedValue(testWorkflow);
    apiMock.listIssueWorkflows.mockResolvedValue({ workflows: [testWorkflow] });
    apiMock.listIssueWorkflowUsage.mockResolvedValue({ usage: [], total: 0 });
    apiMock.listIssueWorkflowTemplates.mockResolvedValue({
      templates: TEMPLATES,
    });
    apiMock.createIssueWorkflow.mockReset();
    apiMock.updateIssueWorkflow.mockReset();
    adapter = makeAdapter();
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
  });

  afterEach(() => {
    cleanup();
    queryClient.clear();
    vi.clearAllMocks();
  });

  const renderComponent = (props: { workflowId?: string; isNew?: boolean } = {}) =>
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <NavigationProvider value={adapter}>
          <WorkflowDesignerPage {...props} />
        </NavigationProvider>
      </QueryClientProvider>,
    );

  it("loads the stored workflow into the designer with its name and branches", async () => {
    renderComponent({ workflowId: "wf-1" });

    expect(await screen.findByDisplayValue("Core Pipeline")).toBeTruthy();
    expect(screen.getByDisplayValue("Development workflow")).toBeTruthy();
    expect(screen.getByTestId("status-node-todo")).toBeTruthy();
    fireEvent.click(screen.getByTestId("status-node-todo"));
    expect(screen.getByText("Start")).toBeTruthy();
    expect(screen.getAllByText("Done").length).toBeGreaterThan(0);
    expect(apiMock.getIssueWorkflow).toHaveBeenCalledWith("wf-1");
  });

  it("starts a new workflow draft when isNew is true", () => {
    renderComponent({ isNew: true });

    expect(screen.getByDisplayValue("New Workflow")).toBeTruthy();
    expect(screen.getByText("Status palette")).toBeTruthy();
    expect(apiMock.getIssueWorkflow).not.toHaveBeenCalled();
  });

  it("keeps the name of the Default workflow read-only", async () => {
    renderComponent({ workflowId: "wf-1" });

    const nameInput = (await screen.findByDisplayValue("Core Pipeline")) as HTMLInputElement;
    expect(nameInput.disabled).toBe(true);
  });

  it("shows the live validation state", async () => {
    renderComponent({ workflowId: "wf-1" });

    expect(await screen.findByText("Workflow is valid")).toBeTruthy();
  });

  it("saves an edit with canonical transitions and no name for the Default workflow", async () => {
    apiMock.updateIssueWorkflow.mockResolvedValue(testWorkflow);
    renderComponent({ workflowId: "wf-1" });
    await screen.findByDisplayValue("Core Pipeline");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(apiMock.updateIssueWorkflow).toHaveBeenCalledTimes(1));
    const [id, body] = apiMock.updateIssueWorkflow.mock.calls[0]!;
    expect(id).toBe("wf-1");
    expect(body).not.toHaveProperty("name");
    expect(body.steps[0]).toEqual({
      status_key: "todo",
      handler: { type: "none" },
      instructions: "",
      transitions: [{ id: "t1", label: "Start", when: null, to_status_key: "in_progress" }],
      layout: { x: 60, y: 60 },
    });
    expect(body.steps[2]).toEqual(
      expect.objectContaining({ status_key: "done", transitions: [] }),
    );
  });

  it("creates a new workflow and opens it", async () => {
    apiMock.createIssueWorkflow.mockResolvedValue({ ...testWorkflow, id: "wf-new", is_default: false });
    renderComponent({ isNew: true });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(apiMock.createIssueWorkflow).toHaveBeenCalledTimes(1));
    expect(apiMock.createIssueWorkflow.mock.calls[0]![0]).toEqual(
      expect.objectContaining({ name: "New Workflow" }),
    );
    await waitFor(() =>
      expect(adapter.replace).toHaveBeenCalledWith(expect.stringContaining("/workflow/wf-new")),
    );
  });

  describe("starting points (blank, clone, template)", () => {
    it("renders starting-point bar with Blank, Clone, and Template when isNew is true", () => {
      renderComponent({ isNew: true });

      expect(screen.getByRole("toolbar", { name: "Starting point" })).toBeTruthy();
      expect(screen.getByRole("button", { name: "Blank" })).toBeTruthy();
      expect(screen.getByRole("button", { name: "Clone an existing workflow" })).toBeTruthy();
      expect(screen.getByRole("button", { name: "Start from a template" })).toBeTruthy();
    });

    it("does not render starting-point bar when editing existing workflow (isNew: false)", async () => {
      renderComponent({ workflowId: "wf-1" });

      await screen.findByDisplayValue("Core Pipeline");
      expect(screen.queryByRole("toolbar", { name: "Starting point" })).toBeNull();
    });

    it("seeds draft immediately from Clone when draft is untouched blank seed", async () => {
      renderComponent({ isNew: true });

      // Open clone dialog
      fireEvent.click(screen.getByRole("button", { name: "Clone an existing workflow" }));

      // Pick workflow from dialog
      const wfItem = await screen.findByText("Core Pipeline");
      fireEvent.click(wfItem);

      // Draft is seeded immediately without discard dialog
      expect(screen.queryByText("Discard unsaved changes?")).toBeNull();
      await waitFor(() =>
        expect(screen.getByDisplayValue("Core Pipeline (Copy)")).toBeTruthy(),
      );
      expect(screen.getByDisplayValue("Development workflow")).toBeTruthy();
    });

    it("seeds draft immediately from Template when draft is untouched blank seed", async () => {
      renderComponent({ isNew: true });

      // Open template dialog
      fireEvent.click(screen.getByRole("button", { name: "Start from a template" }));

      // Pick Delivery template
      const templateItem = await screen.findByText("Delivery");
      fireEvent.click(templateItem);

      // Draft is seeded immediately with template name and description
      expect(screen.queryByText("Discard unsaved changes?")).toBeNull();
      await waitFor(() => expect(screen.getByDisplayValue("Delivery")).toBeTruthy());
      expect(screen.getByDisplayValue("Build, review, then QA before done")).toBeTruthy();
    });

    it("prompts confirmation before replacing when draft has been edited", async () => {
      renderComponent({ isNew: true });

      // Edit name to make draft touched/dirty
      const nameInput = screen.getByDisplayValue("New Workflow");
      fireEvent.change(nameInput, { target: { value: "My Edited Workflow" } });

      // Try to open template dialog and pick a template
      fireEvent.click(screen.getByRole("button", { name: "Start from a template" }));
      const templateItem = await screen.findByText("Content");
      fireEvent.click(templateItem);

      // Discard confirmation dialog appears
      expect(await screen.findByText("Discard unsaved changes?")).toBeTruthy();
      expect(screen.getByText("Your unsaved changes will be lost.")).toBeTruthy();

      // Cancel keeps the edited draft
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(screen.getByDisplayValue("My Edited Workflow")).toBeTruthy();

      // Try again and confirm discard
      fireEvent.click(screen.getByRole("button", { name: "Start from a template" }));
      const templateItemAgain = await screen.findByText("Content");
      fireEvent.click(templateItemAgain);
      fireEvent.click(await screen.findByRole("button", { name: "Discard and replace" }));

      // Now draft is replaced with template
      await waitFor(() => expect(screen.getByDisplayValue("Content")).toBeTruthy());
      expect(screen.getByDisplayValue("Draft, edit and schedule")).toBeTruthy();
    });

    it("resets to blank with confirmation if draft was edited", async () => {
      renderComponent({ isNew: true });

      // Edit description
      const descInput = screen.getByPlaceholderText("Add a description...");
      fireEvent.change(descInput, { target: { value: "Something edited" } });

      // Click Blank button in bar
      fireEvent.click(screen.getByRole("button", { name: "Blank" }));

      // Discard confirmation dialog appears
      expect(await screen.findByText("Discard unsaved changes?")).toBeTruthy();

      // Confirm discard
      fireEvent.click(screen.getByRole("button", { name: "Discard and replace" }));

      // Draft resets to default blank seed
      await waitFor(() => expect(screen.getByDisplayValue("New Workflow")).toBeTruthy());
      expect(screen.getByPlaceholderText("Add a description...")).toHaveValue("");
    });

    it("saves a cloned workflow as a new workflow without is_default, owner_id or project_ids", async () => {
      apiMock.createIssueWorkflow.mockResolvedValue({
        ...testWorkflow,
        id: "wf-cloned",
        name: "Core Pipeline (Copy)",
        is_default: false,
        owner_id: "user-creator",
      });

      renderComponent({ isNew: true });

      // Clone workflow
      fireEvent.click(screen.getByRole("button", { name: "Clone an existing workflow" }));
      const wfItem = await screen.findByText("Core Pipeline");
      fireEvent.click(wfItem);
      await screen.findByDisplayValue("Core Pipeline (Copy)");

      // Click Save
      fireEvent.click(screen.getByRole("button", { name: "Save" }));

      await waitFor(() => expect(apiMock.createIssueWorkflow).toHaveBeenCalledTimes(1));
      const body = apiMock.createIssueWorkflow.mock.calls[0]![0];
      expect(body.name).toBe("Core Pipeline (Copy)");
      expect(body.description).toBe("Development workflow");
      expect(body).not.toHaveProperty("is_default");
      expect(body).not.toHaveProperty("owner_id");
      expect(body).not.toHaveProperty("project_ids");
    });
  });
});
