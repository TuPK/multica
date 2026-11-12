// @vitest-environment jsdom

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssueWorkflow } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { CloneWorkflowDialog } from "./clone-workflow-dialog";

const { apiMock } = vi.hoisted(() => {
  return {
    apiMock: {
      listIssueWorkflows: vi.fn(),
      getIssueWorkflow: vi.fn(),
    },
  };
});

vi.mock("@multica/core/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/api")>();
  return { ...actual, api: apiMock };
});

const sampleWorkflows: IssueWorkflow[] = [
  {
    id: "wf-1",
    workspace_id: "ws-1",
    name: "Engineering Flow",
    description: "Main development pipeline",
    is_default: true,
    owner_id: null,
    steps: [
      { status_key: "todo", handler: { type: "none" }, instructions: "" },
      { status_key: "in_progress", handler: { type: "none" }, instructions: "" },
      { status_key: "done", handler: { type: "none" }, instructions: "" },
      { status_key: "cancelled", handler: { type: "none" }, instructions: "" },
    ],
    project_ids: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "wf-2",
    workspace_id: "ws-1",
    name: "Marketing Review",
    description: "Copy review process",
    is_default: false,
    owner_id: "user-1",
    steps: [
      { status_key: "todo", handler: { type: "none" }, instructions: "" },
      { status_key: "done", handler: { type: "none" }, instructions: "" },
      { status_key: "cancelled", handler: { type: "none" }, instructions: "" },
    ],
    project_ids: [],
    created_at: "2026-01-02T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  },
];

describe("CloneWorkflowDialog", () => {
  let queryClient: QueryClient;
  const onSelectWorkflow = vi.fn();
  const onOpenChange = vi.fn();

  beforeEach(() => {
    onSelectWorkflow.mockReset();
    onOpenChange.mockReset();
    apiMock.listIssueWorkflows.mockResolvedValue({ workflows: sampleWorkflows });
    apiMock.getIssueWorkflow.mockResolvedValue(sampleWorkflows[0]);
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
  });

  afterEach(() => {
    cleanup();
    queryClient.clear();
    vi.clearAllMocks();
  });

  const renderComponent = (open = true) =>
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CloneWorkflowDialog
          open={open}
          onOpenChange={onOpenChange}
          wsId="ws-1"
          onSelectWorkflow={onSelectWorkflow}
        />
      </QueryClientProvider>,
    );

  it("lists existing workflows with name, default badge, and step count", async () => {
    renderComponent();

    expect(await screen.findByText("Engineering Flow")).toBeTruthy();
    expect(screen.getByText("Marketing Review")).toBeTruthy();
    expect(screen.getByText("Default")).toBeTruthy();
    expect(screen.getByText("4 steps")).toBeTruthy();
    expect(screen.getByText("3 steps")).toBeTruthy();
  });

  it("filters workflows based on search input", async () => {
    renderComponent();

    await screen.findByText("Engineering Flow");
    const searchInput = screen.getByPlaceholderText("Search workflows...");

    fireEvent.change(searchInput, { target: { value: "Marketing" } });

    expect(screen.queryByText("Engineering Flow")).toBeNull();
    expect(screen.getByText("Marketing Review")).toBeTruthy();
  });

  it("shows empty state when no workflows match search", async () => {
    renderComponent();

    await screen.findByText("Engineering Flow");
    const searchInput = screen.getByPlaceholderText("Search workflows...");

    fireEvent.change(searchInput, { target: { value: "NonExistent" } });

    expect(screen.getByText("No workflows found")).toBeTruthy();
  });

  it("shows error state when query fails", async () => {
    apiMock.listIssueWorkflows.mockRejectedValue(new Error("Network failure"));
    renderComponent();

    expect(await screen.findByText("Could not load workflows.")).toBeTruthy();
    expect(screen.getByText("Something went wrong. Try again.")).toBeTruthy();
  });

  it("fetches detail and calls onSelectWorkflow when a workflow is clicked", async () => {
    renderComponent();

    const wfItem = await screen.findByText("Engineering Flow");
    fireEvent.click(wfItem);

    await waitFor(() => expect(apiMock.getIssueWorkflow).toHaveBeenCalledWith("wf-1"));
    expect(onSelectWorkflow).toHaveBeenCalledWith(sampleWorkflows[0]);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
