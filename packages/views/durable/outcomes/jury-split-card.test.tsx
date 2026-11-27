/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { JurySplitCard } from "./jury-split-card";

const flag = vi.hoisted(() => ({ on: true }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: () => flag.on,
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

const basePanel = {
  id: "jp-1",
  criterion_id: "c-1",
  juror_agent_ids: ["a-2", "a-3"],
  diversity: "provider",
  state: "split",
  verdict: null,
  resolved_by_type: null,
  created_at: "2026-11-26T09:00:00Z",
  closed_at: null,
  votes: [
    { id: "v-1", verifier_id: "a-2", result: "pass", note: "Matches the acceptance criteria.", created_at: "2026-11-26T09:10:00Z" },
    { id: "v-2", verifier_id: "a-3", result: "fail", note: "Edge case is untested.", created_at: "2026-11-26T09:12:00Z" },
  ],
};

function renderCard() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <JurySplitCard wsId="ws-1" outcomeId="o-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flag.on = true;
  request.mockReset();
});

afterEach(cleanup);

describe("JurySplitCard", () => {
  it("renders nothing and makes no request while the outcomes flag is off", () => {
    flag.on = false;
    const { container } = renderCard();

    expect(container).toBeEmptyDOMElement();
    expect(request).not.toHaveBeenCalled();
  });

  it("shows side-by-side juror votes and reasons for split, escalated, and timed_out panels", async () => {
    request.mockResolvedValueOnce({
      panels: [
        basePanel,
        {
          ...basePanel,
          id: "jp-2",
          state: "escalated",
          diversity: "model_only",
          votes: [
            { id: "v-3", verifier_id: "a-2", result: "unable", note: "Missing staging logs.", created_at: "2026-11-26T09:15:00Z" },
          ],
        },
        {
          ...basePanel,
          id: "jp-3",
          state: "timed_out",
          diversity: "provider",
          votes: [
            { id: "v-4", verifier_id: "a-2", result: "partial", note: "Only one juror finished.", created_at: "2026-11-26T09:20:00Z" },
          ],
        },
      ],
    });

    renderCard();

    expect(await screen.findByText("Jury split")).toBeInTheDocument();
    expect(screen.getByText("Matches the acceptance criteria.")).toBeInTheDocument();
    expect(screen.getByText("Edge case is untested.")).toBeInTheDocument();
    expect(screen.getByText("Passed")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();

    expect(screen.getByText("Escalated for review")).toBeInTheDocument();
    expect(screen.getByText("Different models, same provider")).toBeInTheDocument();
    expect(screen.getByText("Not checked")).toBeInTheDocument();
    expect(screen.getByText("Missing staging logs.")).toBeInTheDocument();

    expect(screen.getByText("Jury timed out")).toBeInTheDocument();
    expect(screen.getByText("Partly passed")).toBeInTheDocument();
    expect(screen.getByText("Only one juror finished.")).toBeInTheDocument();
  });

  it("shows a one-line state without votes for open, unanimous, and resolved panels", async () => {
    request.mockResolvedValueOnce({
      panels: [
        { ...basePanel, id: "jp-open", state: "open", votes: [] },
        { ...basePanel, id: "jp-unanimous", state: "unanimous", verdict: "pass" },
        { ...basePanel, id: "jp-resolved", state: "resolved", verdict: "fail", resolved_by_type: "member" },
      ],
    });

    renderCard();

    expect(await screen.findByText("Jurors are still voting.")).toBeInTheDocument();
    expect(screen.getByText("Unanimous verdict")).toBeInTheDocument();
    expect(screen.getByText("Resolved")).toBeInTheDocument();
    expect(screen.queryByText("Matches the acceptance criteria.")).not.toBeInTheDocument();
    expect(screen.queryByText("Edge case is untested.")).not.toBeInTheDocument();
  });

  it("warns when diversity is none and never shows the raw enum", async () => {
    request.mockResolvedValueOnce({
      panels: [{ ...basePanel, diversity: "none" }],
    });

    renderCard();

    expect(await screen.findByText("All jurors use the same model.")).toBeInTheDocument();
    expect(screen.getByText("Same model")).toBeInTheDocument();
    expect(screen.queryByText("model_only")).not.toBeInTheDocument();
    expect(screen.queryByText("none")).not.toBeInTheDocument();
  });
});
