// @vitest-environment jsdom

import { cleanup, fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssueWorkflowTemplate, ListIssueWorkflowTemplatesResponse } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { TemplatePickerDialog } from "./template-picker-dialog";

const { apiMock } = vi.hoisted(() => {
  return {
    apiMock: {
      listIssueWorkflowTemplates: vi.fn(),
    },
  };
});

vi.mock("@multica/core/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/api")>();
  return { ...actual, api: apiMock };
});

// Fixtures in the shape the server sends: an existing status by key, or a
// status the save creates by name. Each template lists todo, done and cancelled.
const handler = { type: "none" as const };
const existing = (status_key: string, to?: string) => ({
  status_key,
  handler,
  instructions: "",
  transitions: to ? [{ id: `${status_key}-default`, label: "Default", when: null, to_status_key: to }] : [],
});
const created = (name: string, category = "started", to?: string) => ({
  new_status: { name, category },
  handler,
  instructions: "",
  transitions: to ? [{ id: `${name}-default`, label: "Default", when: null, to_status_key: to }] : [],
});

const TEMPLATES: IssueWorkflowTemplate[] = [
  {
    id: "delivery",
    steps: [
      existing("todo", "in_progress"),
      existing("in_progress", "in_review"),
      existing("in_review", "qa"),
      created("QA", "started", "done"),
      existing("done"),
      existing("cancelled"),
    ],
  },
  {
    id: "content",
    steps: [existing("todo", "drafting"), created("Drafting", "started", "done"), existing("done"), existing("cancelled")],
  },
  {
    id: "community",
    steps: [existing("todo", "triage"), created("Triage", "unstarted", "done"), existing("done"), existing("cancelled")],
  },
  {
    id: "sdlc_auto",
    steps: [
      { status_key: "todo", handler, instructions: "", role_hint: "developer", transitions: [] },
      existing("done"),
      existing("cancelled"),
    ],
  },
  {
    id: "bmad_planning_auto",
    steps: [created("Research", "started", "done"), existing("done"), existing("cancelled")],
  },
  {
    id: "sales_loop",
    steps: [created("Scouting", "started", "done"), existing("done"), existing("cancelled")],
  },
  {
    id: "quick_fix",
    steps: [existing("todo", "in_progress"), existing("in_progress", "done"), existing("done"), existing("cancelled")],
  },
];

describe("TemplatePickerDialog", () => {
  let queryClient: QueryClient;
  const onSelectTemplate = vi.fn();
  const onOpenChange = vi.fn();

  beforeEach(() => {
    onSelectTemplate.mockReset();
    onOpenChange.mockReset();
    apiMock.listIssueWorkflowTemplates.mockResolvedValue({
      templates: TEMPLATES,
    } as ListIssueWorkflowTemplatesResponse);
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
        <TemplatePickerDialog
          open={open}
          onOpenChange={onOpenChange}
          wsId="ws-1"
          onSelectTemplate={onSelectTemplate}
        />
      </QueryClientProvider>,
    );

  it("lists the seven templates with translated names, descriptions, and step previews", async () => {
    renderComponent();

    expect(await screen.findByText("Delivery")).toBeTruthy();
    expect(screen.getByText("Build, review, then QA before done")).toBeTruthy();
    expect(screen.getByText("Content")).toBeTruthy();
    expect(screen.getByText("Draft, edit and schedule")).toBeTruthy();
    expect(screen.getByText("Community")).toBeTruthy();
    expect(screen.getByText("Triage, reply, wait on the reporter")).toBeTruthy();
    expect(screen.getByText("SDLC Auto")).toBeTruthy();
    expect(screen.getByText("BMAD Planning Auto")).toBeTruthy();
    expect(screen.getByText("Sales Loop")).toBeTruthy();
    expect(screen.getByText("SDLC Quick Fix")).toBeTruthy();
  });

  it("marks created statuses as will be created on save", async () => {
    renderComponent();

    expect(await screen.findByText("Delivery")).toBeTruthy();
    const badges = screen.getAllByTestId("new-status-badge");
    expect(badges.length).toBeGreaterThan(0);
    expect(badges[0]?.textContent).toBe("Will be created on save");
    expect(screen.getByText("QA")).toBeTruthy();
  });

  it("shows the suggested role on a column", async () => {
    renderComponent();

    expect(await screen.findByText("SDLC Auto")).toBeTruthy();
    expect(screen.getByText("developer")).toBeTruthy();
  });

  it("skips unknown template IDs", async () => {
    apiMock.listIssueWorkflowTemplates.mockResolvedValue({
      templates: [
        ...TEMPLATES,
        { id: "unknown_future_template", steps: [] },
      ],
    });
    renderComponent();

    expect(await screen.findByText("Delivery")).toBeTruthy();
    expect(screen.queryByText("unknown_future_template")).toBeNull();
  });

  it("shows empty state when no templates returned", async () => {
    apiMock.listIssueWorkflowTemplates.mockResolvedValue({ templates: [] });
    renderComponent();

    expect(await screen.findByText("No templates found")).toBeTruthy();
  });

  it("shows error state when query fails", async () => {
    apiMock.listIssueWorkflowTemplates.mockRejectedValue(new Error("Network failure"));
    renderComponent();

    expect(await screen.findByText("Could not load templates.")).toBeTruthy();
    expect(screen.getByText("Something went wrong. Try again.")).toBeTruthy();
  });

  it("calls onSelectTemplate when a template is clicked", async () => {
    renderComponent();

    const deliveryItem = await screen.findByText("Delivery");
    fireEvent.click(deliveryItem);

    expect(onSelectTemplate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "delivery" }),
      "Delivery",
      "Build, review, then QA before done",
    );
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
