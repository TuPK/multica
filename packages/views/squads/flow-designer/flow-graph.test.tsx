// @vitest-environment jsdom

import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc, FlowInsights, FlowIssue } from "@multica/core/types";
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

describe("FlowGraph issue marks", () => {
  const marked = (key: string) => document.querySelector(`[data-step-key="${key}"] svg.text-amber-500`) !== null;

  // The server names a step in a path by its key. A step that only appears in the path of another step's input is
  // not the step the issue is about.
  it("marks the step an issue is about, and not a step that its path only mentions", () => {
    const issue: FlowIssue = {
      code: "input_no_files",
      severity: "warning",
      message: "No file to read",
      path: "steps[gate].tasks[check].inputs[plan.notes]",
    };
    renderGraph({ issues: [issue] });
    expect(marked("gate")).toBe(true);
    expect(marked("plan")).toBe(false);
  });
});

describe("FlowGraph canvas fit", () => {
  // Five steps in a row: the canvas is 4 * 300 + 256 = 1456px wide before the padding.
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
  const CONTENT = 4 * 300 + 256;

  afterEach(() => {
    vi.restoreAllMocks();
    width = 1000;
  });

  it("fits the whole graph in the canvas the first time it is laid out, without a press on Fit", () => {
    sizeOfCanvas();
    renderGraph({ doc: chain });
    expect(scaleOf()).toBeCloseTo(AVAILABLE(1000) / CONTENT, 2);
  });

  it("shows a six-step flow whole in the canvas of a 1280px window, at a size the text can still be read", () => {
    // The page of a 1280px window leaves the canvas about 970px: the sidebar and the page padding take the rest.
    width = 970;
    sizeOfCanvas();
    const six = { ...chain, steps: [...chain.steps, { ...chain.steps[4]!, key: "s6", title: "s6" }].map((step, i, all) => ({
      ...step,
      edges: [{ id: `${step.key}_ok`, label: "Next", to: all[i + 1]?.key ?? "done" }],
    })) } as unknown as FlowDoc;
    renderGraph({ doc: six });
    const scale = scaleOf();
    const lastCard = screen.getByTestId("flow-graph-canvas").querySelector<HTMLElement>('[data-step-key="s6"]')!;
    const drawn = parseFloat(lastCard.style.left) + 256;
    expect(scale).toBeGreaterThanOrEqual(0.5);
    expect(drawn * scale).toBeLessThanOrEqual(AVAILABLE(970) + 0.5);
  });

  it("puts the fitted view on screen at once: a transition would show the graph cut for a moment", () => {
    sizeOfCanvas();
    renderGraph({ doc: chain });
    expect(screen.getByTestId("flow-graph-canvas").className).not.toMatch(/transition/);
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

describe("FlowGraph with 40 steps (K13)", () => {
  // A chain of 40 steps with two loops: s10 goes back to s05 (fixed max) and s30 goes back to s20 (max from a variable).
  const keys = Array.from({ length: 40 }, (_, i) => `s${String(i + 1).padStart(2, "0")}`);
  const big = {
    version: 2,
    start: "s01",
    terminals: [{ key: "done", status: "done" }],
    loops: [
      { key: "fix", header: "s05", max: 3 },
      { key: "polish", header: "s20", max: { var: "rounds", cap: 5 } },
    ],
    steps: keys.map((key, i) => {
      const next = keys[i + 1] ?? "done";
      const edges: Array<Record<string, unknown>> = [{ id: `${key}_ok`, label: "Next", to: next }];
      if (key === "s10") edges.push({ id: "s10_back", label: "Fix", to: "s05", loop: "fix" });
      if (key === "s30") edges.push({ id: "s30_back", label: "Polish", to: "s20", loop: "polish" });
      return { key, title: `Step ${key.slice(1)}`, kind: "lead", edges };
    }),
  } as unknown as FlowDoc;

  const cardsOnCanvas = () => screen.getByTestId("flow-graph-canvas").querySelectorAll<HTMLElement>("[data-step-key]");

  it("draws every step once, and no two cards sit on the same spot", () => {
    renderGraph({ doc: big });
    const cards = [...cardsOnCanvas()];
    expect(cards.map((c) => c.dataset.stepKey).sort()).toEqual([...keys].sort());
    const spots = new Set(cards.map((c) => `${c.style.left},${c.style.top}`));
    expect(spots.size).toBe(40);
  });

  it("draws each back edge dashed and labelled with its loop max", () => {
    renderGraph({ doc: big });
    const back = screen.getAllByTestId("flow-back-edge");
    expect(back.map((e) => e.getAttribute("data-edge-id")).sort()).toEqual(["s10_back", "s30_back"]);
    for (const edge of back) {
      const path = edge.querySelector("path");
      expect(path?.getAttribute("stroke-dasharray") ?? "").not.toBe("");
    }
    const labelOf = (id: string) => back.find((e) => e.getAttribute("data-edge-id") === id)?.querySelector("text")?.textContent;
    expect(labelOf("s10_back")).toBe("↺ 3");
    expect(labelOf("s30_back")).toBe("↺ 5");
  });

  it("selects the step that Find a step names, and no other", () => {
    const onSelectStep = vi.fn();
    renderGraph({ doc: big, onSelectStep });
    const find = screen.getByPlaceholderText("Find a step");
    fireEvent.change(find, { target: { value: "step 27" } });
    fireEvent.keyDown(find, { key: "Enter" });
    expect(onSelectStep).toHaveBeenCalledTimes(1);
    expect(onSelectStep).toHaveBeenCalledWith("s27");

    // A name that no step has selects nothing.
    fireEvent.change(find, { target: { value: "no such step" } });
    fireEvent.keyDown(find, { key: "Enter" });
    expect(onSelectStep).toHaveBeenCalledTimes(1);
  });

  it("keeps the vertical list for phones: all 40 steps in order, shown only below 640px", () => {
    renderGraph({ doc: big });
    const list = screen.getByText("Steps in order").parentElement as HTMLElement;
    expect(list.className).toContain("sm:hidden");
    const rows = [...list.querySelectorAll("div[class*='cursor-pointer']")];
    expect(rows).toHaveLength(40);
    expect(rows[0]?.textContent).toContain("#1 Step 01");
    expect(rows[39]?.textContent).toContain("#40 Step 40");
    expect(screen.getByTestId("flow-graph-canvas").parentElement?.className).toMatch(/\bhidden\b.*\bsm:block\b/);
  });
});
