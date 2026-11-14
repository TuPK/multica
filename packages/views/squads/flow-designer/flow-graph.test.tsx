// @vitest-environment jsdom

import { cleanup, fireEvent, screen } from "@testing-library/react";
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

describe("FlowGraph canvas fit", () => {
  // Five steps in a row: the canvas is 4 * 340 + 256 = 1616px wide before the padding.
  const chain = {
    version: 2,
    start: "s1",
    terminals: [{ key: "done", status: "done" }],
    steps: ["s1", "s2", "s3", "s4", "s5"].map((key, i, all) => ({
      key,
      title: key,
      kind: "lead",
      edges: [{ id: `${key}_ok`, label: "Next", ...(all[i + 1] ? { to: all[i + 1] } : { to: "done" }) }],
    })),
  } as unknown as FlowDoc;

  let width = 1000;
  const sizeOfCanvas = () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(() => width);
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(() => 700);
  };
  const scaleOf = () => {
    const match = /scale\(([\d.]+)\)/.exec(screen.getByTestId("flow-graph-canvas").style.transform);
    return match ? Number(match[1]) : NaN;
  };
  const AVAILABLE = (w: number) => w - 64; // the canvas has 32px of padding on each side
  const CONTENT = 1616;

  afterEach(() => {
    vi.restoreAllMocks();
    width = 1000;
  });

  it("fits the whole graph in the canvas the first time it is laid out, without a press on Fit", () => {
    sizeOfCanvas();
    renderGraph({ doc: chain });
    expect(scaleOf()).toBeCloseTo(AVAILABLE(1000) / CONTENT, 2);
  });

  it("does not enlarge a graph that already fits", () => {
    width = 2400;
    sizeOfCanvas();
    renderGraph({ doc: chain });
    expect(scaleOf()).toBe(1);
  });

  it("does not shrink past half size, so that the text stays readable", () => {
    width = 500;
    sizeOfCanvas();
    renderGraph({ doc: chain });
    expect(scaleOf()).toBe(0.5);
  });

  it("fits once: a later change of the document or the width leaves the view alone, and Fit fits it again", () => {
    sizeOfCanvas();
    const view = renderGraph({ doc: chain });
    const first = scaleOf();

    width = 1400;
    view.rerender(<FlowGraph doc={{ ...chain, steps: [...chain.steps] }} selectedStepKey={null} onSelectStep={vi.fn()} />);
    expect(scaleOf()).toBe(first);

    fireEvent.click(screen.getByRole("button", { name: "Fit" }));
    expect(scaleOf()).toBeCloseTo(AVAILABLE(1400) / CONTENT, 2);
  });
});
