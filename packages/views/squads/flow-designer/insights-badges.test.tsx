// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import { ApiClient } from "@multica/core/api/client";
import { useFlowInsights } from "@multica/core/packs";
import type { FlowInsights } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { EdgeInsightBadge, InsightsBar, StepInsightBadge, insightsHaveData } from "./insights-badges";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const insights = (over: Partial<FlowInsights> = {}): FlowInsights => ({
  requests: { total: 51, open: 5, by_end: { "terminal:done": 40 } },
  steps: [
    { key: "gate", entered: 51, median_minutes: 38, p90_minutes: 190 },
    { key: "plan", entered: 1, median_minutes: null, p90_minutes: null },
  ],
  edges: [{ step: "gate", edge: "gate_fix", taken: 14 }],
  loops: [],
  stalls: [],
  manual_closes: 0,
  truncated: false,
  ...over,
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("insights badges", () => {
  it("shows how often a step was entered and how long a visit took", () => {
    renderWithI18n(<StepInsightBadge insights={insights()} stepKey="gate" />);
    expect(screen.getByText("Entered 51 times")).toBeInTheDocument();
    expect(screen.getByText("median 38 min, p90 190 min")).toBeInTheDocument();
  });

  it("shows the count alone when no visit has a length", () => {
    renderWithI18n(<StepInsightBadge insights={insights()} stepKey="plan" />);
    expect(screen.getByText("Entered 1 time")).toBeInTheDocument();
    expect(screen.queryByText(/median/)).toBeNull();
  });

  it("shows how often an edge was taken, and nothing for an edge nobody took", () => {
    const { rerender } = renderWithI18n(<EdgeInsightBadge insights={insights()} stepKey="gate" edgeId="gate_fix" />);
    expect(screen.getByText("Taken 14 times")).toBeInTheDocument();
    rerender(<EdgeInsightBadge insights={insights()} stepKey="gate" edgeId="gate_ok" />);
    expect(screen.queryByText(/Taken/)).toBeNull();
  });

  it("is hidden when there is no data", () => {
    const empty = insights({ requests: { total: 0, open: 0, by_end: {} }, steps: [], edges: [] });
    expect(insightsHaveData(empty)).toBe(false);
    expect(insightsHaveData(null)).toBe(false);
    const { container } = renderWithI18n(
      <>
        <StepInsightBadge insights={empty} stepKey="gate" />
        <EdgeInsightBadge insights={empty} stepKey="gate" edgeId="gate_fix" />
        <InsightsBar insights={empty} includeTrial={false} onIncludeTrial={vi.fn()} />
        <StepInsightBadge insights={null} stepKey="gate" />
      </>,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("says how many requests the numbers come from, that only the latest are counted, and toggles trial runs", () => {
    const onIncludeTrial = vi.fn();
    renderWithI18n(<InsightsBar insights={insights({ truncated: true })} includeTrial={false} onIncludeTrial={onIncludeTrial} />);
    expect(screen.getByText("Numbers from 51 requests")).toBeInTheDocument();
    expect(screen.getByText("Only the latest 500 requests are counted")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: "Include trial runs" }));
    expect(onIncludeTrial).toHaveBeenCalledWith(true);
  });

  it("does not break on an answer of the server that is not a report: nothing is shown", async () => {
    // The real client, so the answer goes through the schema.
    vi.stubGlobal("fetch", vi.fn().mockImplementation(async () => new Response(JSON.stringify({ requests: "many", steps: 3, edges: [null, 4], loops: {} }), { status: 200, headers: { "Content-Type": "application/json" } })));
    setApiInstance(new ApiClient("https://api.example.test"));
    function Harness() {
      const q = useFlowInsights("wf-1", { includeTrial: false });
      return (
        <div data-testid="harness" data-state={q.isSuccess ? "loaded" : "loading"}>
          <InsightsBar insights={q.data ?? null} includeTrial={false} onIncludeTrial={vi.fn()} />
          <StepInsightBadge insights={q.data ?? null} stepKey="gate" />
          <EdgeInsightBadge insights={q.data ?? null} stepKey="gate" edgeId="gate_fix" />
        </div>
      );
    }
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderWithI18n(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("harness")).toHaveAttribute("data-state", "loaded"));
    expect(screen.getByTestId("harness")).toBeEmptyDOMElement();
  });
});
