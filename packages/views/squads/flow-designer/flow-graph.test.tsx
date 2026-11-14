// @vitest-environment jsdom

import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc, FlowInsights } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { FlowGraph } from "./flow-graph";

const doc: FlowDoc = {
  version: 2,
  start: "plan",
  terminals: [{ key: "done", status: "done" }],
  steps: [
    { key: "plan", title: "Plan", kind: "lead", edges: [{ id: "plan_ok", label: "Planned", to: "gate" }] },
    {
      key: "gate",
      title: "Gate",
      kind: "tasks",
      tasks: [{ key: "check", title: "Check", kind: "gate", role: "checker" }],
      edges: [
        { id: "gate_ok", label: "Passed", to: "done" },
        { id: "gate_fix", label: "Fix", to: "plan" },
      ],
    },
  ],
} as unknown as FlowDoc;

const insights = (over: Partial<FlowInsights> = {}): FlowInsights => ({
  requests: { total: 51, open: 0, by_end: {} },
  steps: [{ key: "gate", entered: 51, median_minutes: 38, p90_minutes: 190 }],
  edges: [{ step: "gate", edge: "gate_fix", taken: 14 }],
  loops: [],
  stalls: [],
  manual_closes: 0,
  truncated: false,
  ...over,
});

const renderGraph = (props: Partial<React.ComponentProps<typeof FlowGraph>> = {}) =>
  renderWithI18n(<FlowGraph doc={doc} selectedStepKey={null} onSelectStep={vi.fn()} {...props} />);

afterEach(cleanup);

describe("FlowGraph insights", () => {
  it("shows the numbers of the version on its steps and its edges", () => {
    renderGraph({ insights: insights() });
    expect(screen.getAllByText("Entered 51 times").length).toBeGreaterThan(0);
    expect(screen.getAllByText("median 38 min, p90 190 min").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Taken 14 times").length).toBeGreaterThan(0);
    // The step nobody entered has no badge.
    expect(screen.queryByText(/Entered 0/)).toBeNull();
  });

  it("shows no badge when the draft has no numbers, or the version has no requests", () => {
    renderGraph();
    expect(screen.queryByText(/Entered/)).toBeNull();
    cleanup();
    renderGraph({ insights: insights({ requests: { total: 0, open: 0, by_end: {} }, steps: [], edges: [] }) });
    expect(screen.queryByText(/Entered|Taken/)).toBeNull();
    cleanup();
    renderGraph({ insights: null });
    expect(screen.queryByText(/Entered|Taken/)).toBeNull();
  });
});
