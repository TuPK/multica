// @vitest-environment jsdom

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssueWorkflow } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { WorkflowDesignerPage } from "./workflow-designer-page";

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
  useIssueStatuses: () => ({
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
  }),
}));

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
});
