/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";
import { AttentionQueueSection } from "./attention-queue-section";

const api = vi.hoisted(() => ({ getQueuedAttention: vi.fn() }));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api,
}));

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <AttentionQueueSection wsId="ws-1" />
    </QueryClientProvider>,
  );
}

describe("AttentionQueueSection", () => {
  beforeEach(() => {
    api.getQueuedAttention.mockReset();
  });
  afterEach(cleanup);

  it("lists queued asks with subject type, reason, estimated minutes, and age", async () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    api.getQueuedAttention.mockResolvedValue({
      asks: [
        {
          subject_type: "proposal",
          subject_id: "p-1",
          reason: "judgment",
          estimated_minutes: 3.5,
          created_at: twoHoursAgo,
        },
        {
          subject_type: "effect_intent",
          subject_id: "i-1",
          reason: "irreversible",
          estimated_minutes: 2,
          created_at: twoHoursAgo,
        },
      ],
    });
    renderSection();

    expect(await screen.findByRole("heading", { name: "Queued" })).toBeInTheDocument();
    expect(await screen.findByText("Proposal")).toBeInTheDocument();
    expect(screen.getByText("Needs judgment")).toBeInTheDocument();
    expect(screen.getByText("3.5 min")).toBeInTheDocument();
    expect(screen.getByText("Action approval")).toBeInTheDocument();
    expect(screen.getByText("Irreversible")).toBeInTheDocument();
    expect(screen.getByText("2 min")).toBeInTheDocument();
    expect(screen.getAllByText(/2h ago|2 hours ago/i).length).toBeGreaterThanOrEqual(1);
  });

  it("shows one short empty line when nothing is queued", async () => {
    api.getQueuedAttention.mockResolvedValue({ asks: [] });
    renderSection();

    expect(await screen.findByText("No questions are queued.")).toBeInTheDocument();
  });

  it("shows an error alert when the request fails", async () => {
    api.getQueuedAttention.mockRejectedValue(new Error("boom"));
    renderSection();

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load queued questions.");
  });
});
