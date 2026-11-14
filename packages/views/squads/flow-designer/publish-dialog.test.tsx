// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import { ApiError, type ApiClient } from "@multica/core/api/client";
import type { FlowDoc } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { PublishDialog } from "./publish-dialog";

const mocks = vi.hoisted(() => ({ navPush: vi.fn() }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({ useWorkspacePaths: () => ({ squads: () => "/acme/squads" }) }));
vi.mock("../../navigation", () => ({ useNavigation: () => ({ push: mocks.navPush }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const fakeApi = { validateFlowDraft: vi.fn(), publishFlowDraft: vi.fn(), createFlowDraft: vi.fn(), setSquadActiveWorkflow: vi.fn() };
const doc = { version: 2, start: "plan", steps: [], terminals: [] } as unknown as FlowDoc;

function renderDialog(props: Partial<React.ComponentProps<typeof PublishDialog>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const saveDraft = vi.fn().mockResolvedValue(4);
  renderWithI18n(
    <QueryClientProvider client={client}>
      <PublishDialog open onOpenChange={vi.fn()} squadId="sq-1" draftId="d-1" doc={doc} isAdmin={false} saveDraft={saveDraft} {...props} />
    </QueryClientProvider>,
  );
  return { saveDraft };
}

const publish = () => screen.getByRole("button", { name: "Publish" });

describe("PublishDialog and the proposal acknowledgement", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.validateFlowDraft.mockResolvedValue({ ok: true, revision: 4, issues: [] });
    fakeApi.publishFlowDraft.mockResolvedValue({ id: "wf-9" });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("asks a draft that an agent proposed for the acknowledgement, and sends it when ticked", async () => {
    renderDialog({ origin: "proposal" });
    const box = await screen.findByRole("checkbox", { name: /i have read this workflow/i });
    expect(publish()).toBeDisabled();

    fireEvent.click(box);
    await waitFor(() => expect(publish()).toBeEnabled());
    fireEvent.click(publish());
    await waitFor(() => expect(fakeApi.publishFlowDraft).toHaveBeenCalledWith("sq-1", "d-1", { revision: 4, ack: ["proposal_reviewed"] }));
  });

  it("lets a member who manages the squad give it, without being an admin", async () => {
    renderDialog({ origin: "proposal", isAdmin: false });
    const box = await screen.findByRole("checkbox", { name: /i have read this workflow/i });
    expect(box).toBeEnabled();
    expect(screen.queryByText(/admin/i)).toBeNull();
  });

  it("does not ask a draft of any other origin", async () => {
    renderDialog({ origin: "manual" });
    await waitFor(() => expect(fakeApi.validateFlowDraft).toHaveBeenCalled());
    await waitFor(() => expect(publish()).toBeEnabled());
    expect(screen.queryByRole("checkbox", { name: /i have read this workflow/i })).toBeNull();
    fireEvent.click(publish());
    await waitFor(() => expect(fakeApi.publishFlowDraft).toHaveBeenCalledWith("sq-1", "d-1", { revision: 4, ack: [] }));
  });

  it("shows the box when the server asks for it though the draft says nothing of its origin", async () => {
    fakeApi.publishFlowDraft.mockRejectedValueOnce(new ApiError("needs an ack", 422, "", { code: "flow_ack_required", required: ["proposal_reviewed"] }));
    renderDialog();
    await waitFor(() => expect(publish()).toBeEnabled());
    fireEvent.click(publish());
    expect(await screen.findByRole("checkbox", { name: /i have read this workflow/i })).toBeInTheDocument();
  });
});
