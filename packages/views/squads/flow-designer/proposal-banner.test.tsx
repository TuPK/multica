// @vitest-environment jsdom

import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import { ApiError, type ApiClient } from "@multica/core/api/client";
import type { Attachment, FlowDraft } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { ProposalBanner } from "./proposal-banner";

const mocks = vi.hoisted(() => ({ toastError: vi.fn(), toastSuccess: vi.fn() }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ issueDetail: (id: string) => `/acme/issues/${id}` }),
}));
vi.mock("../../navigation", () => ({
  AppLink: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: mocks.toastSuccess } }));

const fakeApi = { listAttachments: vi.fn(), importFlowProposal: vi.fn(), listAgents: vi.fn() };

const draft = (over: Partial<FlowDraft> = {}): FlowDraft =>
  ({
    id: "d-1",
    squad_id: "sq-1",
    name: "Intake",
    revision: 1,
    origin: "proposal",
    source_ref: { issue_id: "iss-1", agent_id: "ag-1" },
    updated_at: "2026-11-14T10:00:00Z",
    ...over,
  }) as FlowDraft;

const file = (over: Partial<Attachment> = {}): Attachment =>
  ({ id: "att-1", filename: "flow.json", uploader_type: "agent", uploader_id: "ag-1", created_at: "2026-11-14T11:00:00Z", ...over }) as Attachment;

function renderBanner(d: FlowDraft, props: Partial<React.ComponentProps<typeof ProposalBanner>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const onImported = vi.fn();
  const saveNow = vi.fn().mockResolvedValue(d.revision);
  const result = renderWithI18n(
    <QueryClientProvider client={client}>
      <ProposalBanner squadId="sq-1" draft={d} saveNow={saveNow} onImported={onImported} {...props} />
    </QueryClientProvider>,
  );
  return { onImported, saveNow, container: result.container };
}

describe("ProposalBanner", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.listAgents.mockResolvedValue([{ id: "ag-1", name: "Designer", archived_at: null }]);
    fakeApi.listAttachments.mockResolvedValue([]);
    fakeApi.importFlowProposal.mockResolvedValue(draft({ revision: 2, updated_at: "2026-11-14T12:00:00Z" }));
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("is not there for a draft that nobody proposed", () => {
    const { container } = renderBanner(draft({ origin: "manual", source_ref: null }));
    expect(container).toBeEmptyDOMElement();
    expect(fakeApi.listAttachments).not.toHaveBeenCalled();
  });

  it("says it is waiting for the agent, and links to the issue the agent was given", async () => {
    renderBanner(draft());
    expect(await screen.findByText("Waiting for the agent's proposal")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the issue" })).toHaveAttribute("href", "/acme/issues/iss-1");
    expect(screen.queryByRole("button", { name: "Import proposal" })).toBeNull();
    expect(fakeApi.listAttachments).toHaveBeenCalledWith("iss-1");
  });

  it("keeps waiting while the files on the issue are not a flow.json of the agent", async () => {
    fakeApi.listAttachments.mockResolvedValue([
      file({ id: "a", uploader_type: "member" }),
      file({ id: "b", uploader_id: "ag-2" }),
      file({ id: "c", filename: "notes.md" }),
    ]);
    renderBanner(draft());
    await waitFor(() => expect(fakeApi.listAttachments).toHaveBeenCalled());
    expect(await screen.findByText("Waiting for the agent's proposal")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Import proposal" })).toBeNull();
  });

  it("offers Import proposal once the agent has attached a flow.json", async () => {
    fakeApi.listAttachments.mockResolvedValue([file()]);
    renderBanner(draft());
    expect(await screen.findByRole("button", { name: "Import proposal" })).toBeInTheDocument();
    expect(screen.queryByText("Waiting for the agent's proposal")).toBeNull();
  });

  it("saves what is on screen, imports at the revision the server holds, and hands the draft over", async () => {
    fakeApi.listAttachments.mockResolvedValue([file()]);
    const { onImported, saveNow } = renderBanner(draft());
    saveNow.mockResolvedValue(3);
    fireEvent.click(await screen.findByRole("button", { name: "Import proposal" }));

    await waitFor(() => expect(fakeApi.importFlowProposal).toHaveBeenCalledWith("sq-1", "d-1", { revision: 3 }));
    await waitFor(() => expect(onImported).toHaveBeenCalledWith(expect.objectContaining({ id: "d-1", revision: 2 })));
  });

  it("imports nothing when the draft could not be saved first", async () => {
    fakeApi.listAttachments.mockResolvedValue([file()]);
    const { saveNow, onImported } = renderBanner(draft());
    saveNow.mockResolvedValue(null);
    fireEvent.click(await screen.findByRole("button", { name: "Import proposal" }));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalled());
    expect(fakeApi.importFlowProposal).not.toHaveBeenCalled();
    expect(onImported).not.toHaveBeenCalled();
  });

  it("says who proposed the draft once the file has been read, until a newer file comes", async () => {
    fakeApi.listAttachments.mockResolvedValue([file({ created_at: "2026-11-14T11:00:00Z" })]);
    renderBanner(draft({ revision: 2, updated_at: "2026-11-14T12:00:00Z" }));
    expect(await screen.findByText("Proposed by Designer")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Import proposal" })).toBeNull();

    cleanup();
    fakeApi.listAttachments.mockResolvedValue([file({ created_at: "2026-11-14T13:00:00Z" })]);
    renderBanner(draft({ revision: 2, updated_at: "2026-11-14T12:00:00Z" }));
    expect(await screen.findByRole("button", { name: "Import proposal" })).toBeInTheDocument();
  });

  it("tells the person when the server finds no file, and when the file is not a manifest", async () => {
    fakeApi.listAttachments.mockResolvedValue([file()]);
    renderBanner(draft());
    fakeApi.importFlowProposal.mockRejectedValueOnce(new ApiError("none", 404, "", { code: "flow_proposal_missing" }));
    fireEvent.click(await screen.findByRole("button", { name: "Import proposal" }));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("The agent has not attached a flow.json yet"));

    fakeApi.importFlowProposal.mockRejectedValueOnce(new ApiError("the format is wrong", 400, "", { code: "flow_manifest_invalid" }));
    fireEvent.click(screen.getByRole("button", { name: "Import proposal" }));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("the format is wrong"));
  });

  it("falls back to the waiting line when the files cannot be read", async () => {
    fakeApi.listAttachments.mockRejectedValue(new Error("down"));
    renderBanner(draft());
    expect(await screen.findByText("Waiting for the agent's proposal")).toBeInTheDocument();
  });
});
