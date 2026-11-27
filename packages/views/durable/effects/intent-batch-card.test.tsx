import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import type { Intent } from "@multica/core/durable/intents";
import { renderWithI18n } from "../../test/i18n";
import { IntentBatchCard } from "./intent-batch-card";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function intent(id: string, target: string): Intent {
  return {
    id,
    created_at: "2026-11-25T08:00:00Z",
    expires_at: "2026-11-28T08:00:00Z",
    agent_id: "a-1",
    issue_id: null,
    source: "connected_app",
    target,
    effect_class: "send",
    state: "pending",
    call: null,
    call_sha256: "h",
    preview: { kind: "generic", title: target, body_markdown: "", fields: [], recipients: [], amount_micros: null },
    rationale: "",
    decision_note: "",
    batch_key: "a-1|gmail|send|2026-11-25",
  };
}

const request = vi.mocked(api.request);

function renderBatch(intents: Intent[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <IntentBatchCard intents={intents} />
    </QueryClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => request.mockReset());

describe("IntentBatchCard", () => {
  it("shows each email's recipients and full content in its own row", () => {
    const first = intent("i-1", "gmail:first");
    first.preview = { ...first.preview, kind: "email", recipients: ["first@example.com"], body_markdown: "First line\n  Second line", fields: [{ label: "Subject", before: "Old", after: "New" }] };
    const second = intent("i-2", "gmail:second");
    second.preview = { ...second.preview, kind: "email", recipients: ["second@example.com"], body_markdown: "Another email body" };
    renderBatch([first, second]);

    const rows = screen.getAllByRole("listitem");
    expect(within(rows[0]!).getByText("first@example.com")).toBeInTheDocument();
    expect(within(rows[0]!).getByText("First line\n  Second line", { normalizer: (text) => text })).toHaveClass("whitespace-pre-wrap");
    expect(within(rows[0]!).getByText("Old")).toBeInTheDocument();
    expect(within(rows[0]!).getByText("New")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("second@example.com")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("Another email body")).toBeInTheDocument();
  });

  it("titles the batch with its count and lists each action", () => {
    renderBatch([intent("i-1", "gmail:first"), intent("i-2", "gmail:second")]);

    expect(screen.getByText("2 actions of the same kind")).toBeInTheDocument();
    expect(screen.getByText("gmail:first")).toBeInTheDocument();
    expect(screen.getByText("gmail:second")).toBeInTheDocument();
  });

  it("approves every action in one request and reports the count", async () => {
    request.mockResolvedValueOnce({
      results: [
        { id: "i-1", state: "executed", error: "", status: 200 },
        { id: "i-2", state: "executed", error: "", status: 200 },
      ],
    });
    renderBatch([intent("i-1", "gmail:first"), intent("i-2", "gmail:second")]);

    fireEvent.click(screen.getByRole("button", { name: "Approve all" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("2 actions approved."));
    expect(request).toHaveBeenCalledWith("/api/intents/approve-batch", expect.objectContaining({ method: "POST" }));
    expect(JSON.parse(String((request.mock.calls[0]![1] as RequestInit).body))).toEqual({ ids: ["i-1", "i-2"] });
  });

  it("reports a partial approval with what did not go through", async () => {
    request.mockResolvedValueOnce({
      results: [
        { id: "i-1", state: "executed", error: "", status: 200 },
        { id: "i-2", state: "", error: "already decided", status: 409 },
      ],
    });
    renderBatch([intent("i-1", "gmail:first"), intent("i-2", "gmail:second")]);

    fireEvent.click(screen.getByRole("button", { name: "Approve all" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("1 approved, 1 could not be approved."));
  });

  it("still reports the outcome after the card unmounts before the request settles", async () => {
    let finish!: (value: unknown) => void;
    request.mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)));
    const { unmount } = renderBatch([intent("i-1", "gmail:first"), intent("i-2", "gmail:second")]);

    fireEvent.click(screen.getByRole("button", { name: "Approve all" }));
    await waitFor(() => expect(request).toHaveBeenCalled());
    unmount();
    await act(async () =>
      finish({ results: [{ id: "i-1", state: "executed", error: "", status: 200 }, { id: "i-2", state: "executed", error: "", status: 200 }] }),
    );

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("2 actions approved."));
  });
});
