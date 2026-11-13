// @vitest-environment jsdom

import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import { ApiError, type ApiClient } from "@multica/core/api/client";
import type { IssueWorkflow, Squad } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { kitVietSource, StartDialog } from "./start-dialog";

const mocks = vi.hoisted(() => ({ navPush: vi.fn(), toastError: vi.fn() }));
const fakeApi = { createFlowDraft: vi.fn() };

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("../../navigation", () => ({ useNavigation: () => ({ push: mocks.navPush }) }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ squads: () => "/acme/squads", squadDetail: (id: string) => `/acme/squads/${id}` }),
}));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: vi.fn() } }));

const squad = { id: "sq-1", name: "Design Squad", template_key: "kv_design", active_workflow_id: "wf-1" } as unknown as Squad;
const catalogRow = { id: "wf-1", name: "Design · abc", template_ref: "flow_kv_design@abc", flow_config: { version: 1 } } as unknown as IssueWorkflow;
const plainRow = { id: "wf-2", name: "Plain", template_ref: null, flow_config: null } as unknown as IssueWorkflow;

function renderDialog(props: Partial<React.ComponentProps<typeof StartDialog>> = {}) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <StartDialog open onOpenChange={vi.fn()} squad={squad} workflows={[catalogRow]} {...props} />
    </QueryClientProvider>,
  );
}

describe("StartDialog", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.createFlowDraft.mockResolvedValue({ id: "draft-start-1" });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("offers a blank draft, a copy of the Kit Việt workflow and an import, and no way to ask an agent yet", () => {
    renderDialog();

    expect(screen.getByRole("button", { name: /blank/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /copy a kit việt workflow/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /import a file/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /ask an agent to draft/i })).toBeNull();
  });

  it("hides the copy when the squad has no workflow made from the catalog", () => {
    renderDialog({ workflows: [plainRow] });
    expect(screen.queryByRole("button", { name: /copy a kit việt workflow/i })).toBeNull();
  });

  it("creates a blank draft named after the squad", async () => {
    renderDialog();

    fireEvent.click(screen.getByRole("button", { name: /blank/i }));

    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("sq-1", { name: "Design Squad", from: { kind: "blank" } }));
    await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/sq-1/workflows/draft-start-1"));
  });

  it("cuts a long name to the 56 characters a draft name may have", async () => {
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText("Workflow name"), { target: { value: "x".repeat(80) } });

    fireEvent.click(screen.getByRole("button", { name: /blank/i }));

    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalled());
    expect(fakeApi.createFlowDraft.mock.calls[0]![1].name).toHaveLength(56);
  });

  it("copies the workflow the squad runs, by its id, and lets the server name the copy", async () => {
    renderDialog({ workflows: [plainRow, catalogRow] });

    fireEvent.click(screen.getByRole("button", { name: /copy a kit việt workflow/i }));

    await waitFor(() =>
      expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("sq-1", { from: { kind: "workflow", workflow_id: "wf-1" } }),
    );
  });

  it("says when the draft could not be created and stays open", async () => {
    const onOpenChange = vi.fn();
    fakeApi.createFlowDraft.mockRejectedValue(new ApiError("the squad has the most drafts it may have", 409, "", { code: "flow_limit_reached" }));
    renderDialog({ onOpenChange });

    fireEvent.click(screen.getByRole("button", { name: /blank/i }));

    await waitFor(() => expect(mocks.toastError).toHaveBeenCalled());
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(mocks.navPush).not.toHaveBeenCalled();
  });

  it("calls onOpenImport when clicking Import", () => {
    const onOpenChange = vi.fn();
    const onOpenImport = vi.fn();
    renderDialog({ onOpenChange, onOpenImport });

    fireEvent.click(screen.getByRole("button", { name: /import a file/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onOpenImport).toHaveBeenCalled();
  });
});

describe("kitVietSource", () => {
  it("prefers the catalog workflow the squad runs, else the first one", () => {
    const other = { ...catalogRow, id: "wf-3" } as IssueWorkflow;
    expect(kitVietSource([other, catalogRow], "wf-1")?.id).toBe("wf-1");
    expect(kitVietSource([other, catalogRow], null)?.id).toBe("wf-3");
    expect(kitVietSource([plainRow], null)).toBeNull();
  });
});
