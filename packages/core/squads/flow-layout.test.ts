// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  NODE_HEIGHT,
  NODE_WIDTH,
  layoutFlowGraph,
  type FlowLayoutInputStep,
} from "./flow-layout";

describe("layoutFlowGraph", () => {
  it("layers follow the longest path after removing back edges", () => {
    // start -> a -> c
    // start -> b -> d -> c
    // Longest path to c should be 3 (start[0] -> b[1] -> d[2] -> c[3])
    const steps: FlowLayoutInputStep[] = [
      {
        key: "start",
        edges: [
          { id: "e1", to: "a" },
          { id: "e2", to: "b" },
        ],
      },
      {
        key: "a",
        edges: [{ id: "e3", to: "c" }],
      },
      {
        key: "b",
        edges: [{ id: "e4", to: "d" }],
      },
      {
        key: "d",
        edges: [{ id: "e5", to: "c" }],
      },
      {
        key: "c",
        edges: [{ id: "e6", stop: "grounds_missing" }],
      },
    ];

    const result = layoutFlowGraph({ start: "start", steps });
    expect(result.positions["start"]?.layer).toBe(0);
    expect(result.positions["a"]?.layer).toBe(1);
    expect(result.positions["b"]?.layer).toBe(1);
    expect(result.positions["d"]?.layer).toBe(2);
    expect(result.positions["c"]?.layer).toBe(3);
  });

  it("back edges do not change layers", () => {
    // Same graph as above, but with a back edge c -> a (loop: "fix") and d -> start (loop: "retry")
    const stepsWithBackEdges: FlowLayoutInputStep[] = [
      {
        key: "start",
        edges: [
          { id: "e1", to: "a" },
          { id: "e2", to: "b" },
        ],
      },
      {
        key: "a",
        edges: [{ id: "e3", to: "c" }],
      },
      {
        key: "b",
        edges: [{ id: "e4", to: "d" }],
      },
      {
        key: "d",
        edges: [
          { id: "e5", to: "c" },
          { id: "e_back1", to: "start", loop: "retry" },
        ],
      },
      {
        key: "c",
        edges: [
          { id: "e6", stop: "grounds_missing" },
          { id: "e_back2", to: "a", loop: "fix" },
        ],
      },
    ];

    const result = layoutFlowGraph({ start: "start", steps: stepsWithBackEdges });
    // Layers should be unchanged!
    expect(result.positions["start"]?.layer).toBe(0);
    expect(result.positions["a"]?.layer).toBe(1);
    expect(result.positions["b"]?.layer).toBe(1);
    expect(result.positions["d"]?.layer).toBe(2);
    expect(result.positions["c"]?.layer).toBe(3);
  });

  it("40 steps do not overlap", () => {
    // Generate a 40-step complex flow with parallel branches, merges, and back edges
    const steps: FlowLayoutInputStep[] = [];
    const totalSteps = 40;

    for (let i = 0; i < totalSteps; i++) {
      const key = `step_${i}`;
      const edges: FlowLayoutInputStep["edges"] = [];

      // Add forward edges
      if (i + 1 < totalSteps) {
        edges.push({ id: `e_${i}_${i + 1}`, to: `step_${i + 1}` });
      }
      if (i + 2 < totalSteps) {
        edges.push({ id: `e_${i}_${i + 2}`, to: `step_${i + 2}` });
      }
      // Add back edge every 5 steps
      if (i > 5 && i % 5 === 0) {
        edges.push({ id: `back_${i}`, to: `step_${i - 3}`, loop: `loop_${i}` });
      }

      steps.push({ key, edges });
    }

    const result = layoutFlowGraph({ start: "step_0", steps });
    expect(result.nodes).toHaveLength(40);

    // Verify no two node bounding boxes intersect
    // A node rectangle at (x, y) covers [x, x + NODE_WIDTH] and [y, y + NODE_HEIGHT]
    const nodes = result.nodes;
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i]!;
        const b = nodes[j]!;

        const aLeft = a.position.x;
        const aRight = a.position.x + NODE_WIDTH;
        const aTop = a.position.y;
        const aBottom = a.position.y + NODE_HEIGHT;

        const bLeft = b.position.x;
        const bRight = b.position.x + NODE_WIDTH;
        const bTop = b.position.y;
        const bBottom = b.position.y + NODE_HEIGHT;

        const overlapsX = aLeft < bRight && aRight > bLeft;
        const overlapsY = aTop < bBottom && aBottom > bTop;
        const overlaps = overlapsX && overlapsY;

        expect(
          overlaps,
          `Node ${a.id} at (${aLeft}, ${aTop}) overlaps with node ${b.id} at (${bLeft}, ${bTop})`,
        ).toBe(false);
      }
    }
  });

  it("output is stable when declaration order changes", () => {
    const steps: FlowLayoutInputStep[] = [
      { key: "plan", edges: [{ id: "e1", to: "research" }, { id: "e2", to: "draft" }] },
      { key: "research", edges: [{ id: "e3", to: "review" }] },
      { key: "draft", edges: [{ id: "e4", to: "review" }] },
      { key: "review", edges: [{ id: "e5", to: "release" }, { id: "e6", to: "draft", loop: "fix" }] },
      { key: "release", edges: [{ id: "e7", stop: "done" }] },
    ];

    const result1 = layoutFlowGraph({ start: "plan", steps });

    // Reverse order
    const reversedSteps = [...steps].reverse();
    const result2 = layoutFlowGraph({ start: "plan", steps: reversedSteps });

    // Shuffled order
    const shuffledSteps: FlowLayoutInputStep[] = [steps[3]!, steps[0]!, steps[4]!, steps[1]!, steps[2]!];
    const result3 = layoutFlowGraph({ start: "plan", steps: shuffledSteps });

    for (const key of ["plan", "research", "draft", "review", "release"]) {
      expect(result2.positions[key]).toEqual(result1.positions[key]);
      expect(result3.positions[key]).toEqual(result1.positions[key]);
    }
  });
});
