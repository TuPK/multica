import { screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { PendingIntents } from "./pending-intents";

const { flag } = vi.hoisted(() => ({ flag: { on: false } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/feature-flags", () => ({ useFlag: () => flag.on }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

const pendingRow = (id: string, batchKey = "") => ({
  id,
  created_at: "2026-11-25T08:00:00Z",
  expires_at: "2026-11-28T08:00:00Z",
  agent_id: "a-1",
  issue_id: null,
  source: "connected_app",
  target: `gmail:${id}`,
  effect_class: "send",
  state: "pending",
  call: null,
  call_sha256: "h",
  preview: { kind: "generic", title: `Preview ${id}`, body_markdown: "", fields: [], recipients: [], amount_micros: null },
  rationale: "",
  decision_note: "",
  batch_key: batchKey,
});

function renderSection() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <PendingIntents />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flag.on = false;
  request.mockReset();
});
afterEach(() => request.mockReset());

describe("PendingIntents", () => {
  it("shows nothing and asks for nothing while durable effects is off", async () => {
    const { container } = renderSection();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(request).not.toHaveBeenCalled();
  });

  it("lists the pending decisions under one heading, batches and single cards alike", async () => {
    flag.on = true;
    request.mockResolvedValueOnce({
      intents: [pendingRow("i-1", "k"), pendingRow("i-2", "k"), pendingRow("i-3")],
    });
    renderSection();

    expect(await screen.findByRole("heading", { name: "Waiting for approval" })).toBeInTheDocument();
    expect(screen.getByText("2 actions of the same kind")).toBeInTheDocument();
    expect(screen.getByText("Preview i-3")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith("/api/intents?state=pending");
  });

  it("shows nothing when there is no pending decision", async () => {
    flag.on = true;
    request.mockResolvedValueOnce({ intents: [] });
    const { container } = renderSection();
    await waitFor(() => expect(request).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
