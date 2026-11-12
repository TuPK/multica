// @vitest-environment jsdom

import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssueWorkflow } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { WorkflowManagePage } from "./workflow-manage-page";

const { apiMock, testWorkflows, testUsage } = vi.hoisted(() => {
  const testWorkflows = [
    {
      id: "wf-default",
      workspace_id: "ws-1",
      name: "Default Workflow",
      description: "Standard delivery pipeline",
      is_default: true,
      owner_id: null,
      steps: [
        { status_key: "todo", handler: { type: "none" }, instructions: "", transitions: [] },
        {
          status_key: "in_progress",
          handler: { type: "agent", id: "agent-1" },
          instructions: "",
          transitions: [],
        },
        { status_key: "done", handler: { type: "none" }, instructions: "", transitions: [] },
        { status_key: "cancelled", handler: { type: "none" }, instructions: "", transitions: [] },
      ],
      project_ids: ["p-1"],
      created_at: "2026-10-01T00:00:00Z",
      updated_at: "2026-10-15T00:00:00Z",
    },
    {
      id: "wf-custom",
      workspace_id: "ws-1",
      name: "Custom QA Workflow",
      description: "For QA testing squads",
      is_default: false,
      owner_id: "user-1",
      steps: [
        { status_key: "todo", handler: { type: "none" }, instructions: "", transitions: [] },
        { status_key: "done", handler: { type: "none" }, instructions: "", transitions: [] },
        { status_key: "cancelled", handler: { type: "none" }, instructions: "", transitions: [] },
      ],
      project_ids: [],
      created_at: "2026-10-02T00:00:00Z",
      updated_at: "2026-10-16T00:00:00Z",
    },
  ] as IssueWorkflow[];
  const testUsage = [
    { workflow_id: "wf-default", project_ids: ["p-1"], squad_ids: [], agent_ids: ["agent-1"] },
    { workflow_id: "wf-custom", project_ids: [], squad_ids: [], agent_ids: [] },
  ];
  return {
    testWorkflows,
    testUsage,
    apiMock: {
      listIssueWorkflows: vi.fn(),
      listIssueWorkflowUsage: vi.fn(),
      createIssueWorkflow: vi.fn(),
      deleteIssueWorkflow: vi.fn(),
      listProjects: vi.fn(),
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

vi.mock("@multica/core/issue-statuses/hooks", () => ({
  useIssueStatuses: () => ({ statuses: [] }),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({
    getActorName: (_t: string, id: string) => `Actor-${id}`,
    getActorInitials: (_t: string, _id: string) => "A",
    getActorAvatarUrl: () => null,
  }),
}));

function makeAdapter(overrides: Partial<NavigationAdapter> = {}): NavigationAdapter {
  return {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    prefetch: vi.fn(),
    pathname: "/acme/workflow",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (p) => p,
    ...overrides,
  };
}

/** The card for a workflow, found by its name. */
function cardFor(name: string): HTMLElement {
  return screen.getByText(name).closest('[role="button"]') as HTMLElement;
}

describe("WorkflowManagePage", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    apiMock.listIssueWorkflows.mockResolvedValue({ workflows: testWorkflows });
    apiMock.listIssueWorkflowUsage.mockResolvedValue({ usage: testUsage, total: testUsage.length });
    apiMock.listProjects.mockResolvedValue({
      projects: [{ id: "p-1", title: "Platform", workflow_id: "wf-default" }],
    });
    apiMock.createIssueWorkflow.mockReset();
    apiMock.deleteIssueWorkflow.mockReset();
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
  });

  afterEach(() => {
    cleanup();
    queryClient.clear();
    vi.clearAllMocks();
  });

  const renderComponent = () =>
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <NavigationProvider value={makeAdapter()}>
          <WorkflowManagePage />
        </NavigationProvider>
      </QueryClientProvider>,
    );

  it("renders the workflows from the server with step counts and assignments", async () => {
    renderComponent();

    expect(await screen.findByText("Default Workflow")).toBeTruthy();
    expect(screen.getByText("Custom QA Workflow")).toBeTruthy();
    expect(screen.getByText("Default")).toBeTruthy();
    expect(screen.getByText("4 steps")).toBeTruthy();
    expect(screen.getByText("3 steps")).toBeTruthy();
    expect(screen.getByText("Platform")).toBeTruthy();
  });

  it("filters workflows by search query", async () => {
    renderComponent();
    await screen.findByText("Default Workflow");

    fireEvent.change(screen.getByPlaceholderText("Search workflows..."), {
      target: { value: "QA" },
    });

    expect(screen.queryByText("Default Workflow")).toBeNull();
    expect(screen.getByText("Custom QA Workflow")).toBeTruthy();
  });

  it("filters workflows by category tab using the server usage", async () => {
    renderComponent();
    await screen.findByText("Default Workflow");

    fireEvent.click(screen.getByRole("button", { name: "Projects" }));

    expect(screen.getByText("Default Workflow")).toBeTruthy();
    expect(screen.queryByText("Custom QA Workflow")).toBeNull();
  });

  it("opens the detail drawer when a workflow card is clicked", async () => {
    renderComponent();
    fireEvent.click(await screen.findByText("Custom QA Workflow"));

    expect(screen.getByText("Workflow details")).toBeTruthy();
    expect(screen.getByText("Open designer")).toBeTruthy();
  });

  it("explains that the default workflow cannot be deleted and sends no delete", async () => {
    renderComponent();
    await screen.findByText("Default Workflow");

    fireEvent.click(within(cardFor("Default Workflow")).getByLabelText("Actions"));
    fireEvent.click(screen.getByText("Delete"));

    expect(screen.getByText("Cannot delete workflow")).toBeTruthy();
    expect(screen.getByText(/The workspace default workflow cannot be deleted/i)).toBeTruthy();
    expect(apiMock.deleteIssueWorkflow).not.toHaveBeenCalled();
  });

  it("duplicates a workflow through the create endpoint with its steps", async () => {
    apiMock.createIssueWorkflow.mockResolvedValue({
      ...testWorkflows[1],
      id: "wf-copy",
      name: "Custom QA Workflow (Copy)",
      steps: testWorkflows[1]!.steps,
    });
    renderComponent();
    await screen.findByText("Custom QA Workflow");

    fireEvent.click(within(cardFor("Custom QA Workflow")).getByLabelText("Actions"));
    fireEvent.click(screen.getByText("Duplicate"));

    expect(await screen.findByText("Custom QA Workflow (Copy)")).toBeTruthy();
    expect(apiMock.createIssueWorkflow).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Custom QA Workflow (Copy)",
        description: "For QA testing squads",
        steps: [
          expect.objectContaining({ status_key: "todo", transitions: [] }),
          expect.objectContaining({ status_key: "done" }),
          expect.objectContaining({ status_key: "cancelled" }),
        ],
      }),
    );
  });

  it("deletes only after the user confirms", async () => {
    apiMock.deleteIssueWorkflow.mockResolvedValue(undefined);
    renderComponent();
    await screen.findByText("Custom QA Workflow");

    fireEvent.click(within(cardFor("Custom QA Workflow")).getByLabelText("Actions"));
    fireEvent.click(screen.getByText("Delete"));

    expect(apiMock.deleteIssueWorkflow).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(apiMock.deleteIssueWorkflow).toHaveBeenCalledWith("wf-custom"));
    await waitFor(() => expect(screen.queryByText("Custom QA Workflow")).toBeNull());
  });

  it("shows the reasons the server refuses a delete with", async () => {
    const { ApiError } = await import("@multica/core/api");
    apiMock.deleteIssueWorkflow.mockRejectedValue(
      new ApiError("conflict", 409, "Conflict", {
        error: "2 project(s) still use this workflow",
        code: "workflow_in_use",
        reasons: [
          { code: "workflow_in_use", count: 2, ids: ["p-1", "p-2"], message: "2 project(s) still use this workflow" },
        ],
      }),
    );
    renderComponent();
    await screen.findByText("Custom QA Workflow");

    fireEvent.click(within(cardFor("Custom QA Workflow")).getByLabelText("Actions"));
    fireEvent.click(screen.getByText("Delete"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Cannot delete workflow")).toBeTruthy();
    expect(screen.getByText(/This workflow is currently used by 2 project/)).toBeTruthy();
  });
});
