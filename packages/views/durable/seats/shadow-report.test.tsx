/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { ShadowReport } from "./shadow-report";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

function renderReport() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <ShadowReport wsId="ws-1" agentId="a-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  request.mockReset();
});

afterEach(cleanup);

describe("ShadowReport", () => {
  it("renders shadow run numbers, pairs progress, Wilson lower bound, and average costs", async () => {
    request.mockResolvedValueOnce({
      runs: [
        {
          id: "sr-1",
          agent_id: "a-1",
          candidate_runtime_id: "rt-2",
          candidate_model: "claude-sonnet-4",
          sample_rate: 0.3,
          target_pairs: 20,
          auto_promote: false,
          state: "completed",
          purpose: "model_trial",
          work_class_prefix: "",
          created_at: "2026-11-26T09:00:00Z",
          completed_at: "2026-11-27T01:00:00Z",
          report: {
            pairs: 20,
            shadow: 11,
            tie: 6,
            primary: 3,
            unable: 2,
            not_losing_lower: 0.64,
            primary_cost_avg_micros: 1_250_000,
            shadow_cost_avg_micros: 750_000,
          },
        },
        {
          id: "sr-2",
          agent_id: "a-1",
          candidate_runtime_id: "rt-3",
          candidate_model: "",
          sample_rate: 0.5,
          target_pairs: 10,
          auto_promote: false,
          state: "running",
          purpose: "model_trial",
          work_class_prefix: "",
          created_at: "2026-11-27T02:00:00Z",
          completed_at: null,
          report: {
            pairs: 4,
            shadow: 2,
            tie: 1,
            primary: 1,
            unable: 0,
            not_losing_lower: 0.3,
            primary_cost_avg_micros: 0,
            shadow_cost_avg_micros: 0,
          },
        },
      ],
    });

    renderReport();

    expect(await screen.findByText("claude-sonnet-4")).toBeInTheDocument();
    expect(screen.getByText("Completed")).toBeInTheDocument();
    expect(screen.getByText("20 / 20 pairs")).toBeInTheDocument();
    expect(screen.getByText("Shadow: 11")).toBeInTheDocument();
    expect(screen.getByText("Tie: 6")).toBeInTheDocument();
    expect(screen.getByText("Primary: 3")).toBeInTheDocument();
    expect(screen.getByText("Unable: 2")).toBeInTheDocument();
    expect(screen.getByText("Not-losing lower bound: 64%")).toBeInTheDocument();
    expect(screen.getByText("Primary avg cost: $1.25")).toBeInTheDocument();
    expect(screen.getByText("Shadow avg cost: $0.75")).toBeInTheDocument();

    expect(screen.getByText("Default runtime model")).toBeInTheDocument();
    expect(screen.getByText("Running")).toBeInTheDocument();
    expect(screen.getByText("4 / 10 pairs")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/shadow");
  });

  it("renders an empty state when there are no runs", async () => {
    request.mockResolvedValueOnce({ runs: [] });
    renderReport();

    expect(await screen.findByText("No shadow runs yet.")).toBeInTheDocument();
  });

  it("renders an error alert when loading fails", async () => {
    request.mockRejectedValueOnce(new Error("boom"));
    renderReport();

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load shadow runs.");
  });
});
