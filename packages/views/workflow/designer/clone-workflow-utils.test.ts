// @vitest-environment node

import { describe, expect, it } from "vitest";
import type { IssueWorkflowStep } from "@multica/core/types";
import {
  cloneWorkflowSteps,
  deriveStepOrder,
  ensureStepLayouts,
  getAutoLayoutPosition,
  isUntouchedBlankSeed,
  newTransitionId,
  resolveCreatedStatusKey,
  slugifyStatusName,
} from "./clone-workflow-utils";

describe("clone-workflow-utils", () => {
  const sampleSteps: IssueWorkflowStep[] = [
    {
      status_key: "todo",
      handler: { type: "none" },
      instructions: "start instructions",
      transitions: [
        {
          id: "t-1",
          label: "Start",
          when: null,
          to_status_key: "in_progress",
        },
      ],
    },
    {
      status_key: "in_progress",
      handler: { type: "agent", id: "agent-1" },
      instructions: "code",
      transitions: [
        {
          id: "t-2",
          label: "Review",
          when: { field: "outcome", op: "eq", value: "rejected" },
          to_status_key: "in_progress",
        },
        {
          id: "t-3",
          label: "Done",
          when: null,
          to_status_key: "done",
        },
      ],
    },
    {
      status_key: "done",
      handler: { type: "none" },
      instructions: "",
      transitions: [],
    },
    {
      status_key: "cancelled",
      handler: { type: "none" },
      instructions: "",
      transitions: [],
    },
  ];

  describe("newTransitionId", () => {
    it("generates unique string ids", () => {
      const id1 = newTransitionId();
      const id2 = newTransitionId();
      expect(id1).toMatch(/^trans-/);
      expect(id2).toMatch(/^trans-/);
      expect(id1).not.toBe(id2);
    });
  });

  describe("cloneWorkflowSteps", () => {
    it("creates a deep copy with no shared object references", () => {
      const cloned = cloneWorkflowSteps(sampleSteps);

      expect(cloned).not.toBe(sampleSteps);
      expect(cloned.length).toBe(sampleSteps.length);

      // Verify steps are distinct objects
      expect(cloned[0]).not.toBe(sampleSteps[0]);
      expect(cloned[0]?.handler).not.toBe(sampleSteps[0]?.handler);
      expect(cloned[0]?.transitions).not.toBe(sampleSteps[0]?.transitions);

      // Verify transitions are distinct objects
      const sourceT0 = sampleSteps[0]?.transitions?.[0];
      const clonedT0 = cloned[0]?.transitions?.[0];
      expect(clonedT0).not.toBe(sourceT0);
      expect(clonedT0?.to_status_key).toBe("in_progress");
      expect(clonedT0?.label).toBe("Start");

      // Verify condition was deep cloned
      const sourceT2 = sampleSteps[1]?.transitions?.[0];
      const clonedT2 = cloned[1]?.transitions?.[0];
      expect(clonedT2?.when).toEqual(sourceT2?.when);
      expect(clonedT2?.when).not.toBe(sourceT2?.when);

      // Mutating cloned step does not affect source
      if (cloned[0]) {
        cloned[0].instructions = "mutated";
      }
      expect(sampleSteps[0]?.instructions).toBe("start instructions");
    });

    it("assigns fresh unique transition IDs across all steps", () => {
      const cloned = cloneWorkflowSteps(sampleSteps);
      const allTransitionIds = cloned.flatMap((s) => (s.transitions ?? []).map((t) => t.id));

      expect(allTransitionIds.length).toBe(3);
      const uniqueIds = new Set(allTransitionIds);
      expect(uniqueIds.size).toBe(allTransitionIds.length);

      // Source transition IDs are replaced
      expect(allTransitionIds).not.toContain("t-1");
      expect(allTransitionIds).not.toContain("t-2");
      expect(allTransitionIds).not.toContain("t-3");
    });
  });

  describe("isUntouchedBlankSeed", () => {
    const blankSeedSteps: IssueWorkflowStep[] = [
      {
        status_key: "todo",
        handler: { type: "none" },
        instructions: "",
        transitions: [
          {
            id: "todo-default",
            label: "Start",
            when: null,
            to_status_key: "in_progress",
          },
        ],
      },
      {
        status_key: "in_progress",
        handler: { type: "none" },
        instructions: "",
        transitions: [
          {
            id: "in_progress-done",
            label: "Done",
            when: null,
            to_status_key: "done",
          },
        ],
      },
      {
        status_key: "done",
        handler: { type: "none" },
        instructions: "",
        transitions: [],
      },
      {
        status_key: "cancelled",
        handler: { type: "none" },
        instructions: "",
        transitions: [],
      },
    ];

    it("returns true for untouched blank seed", () => {
      expect(
        isUntouchedBlankSeed({
          name: "New Workflow",
          description: "",
          steps: blankSeedSteps,
          defaultName: "New Workflow",
          defaultBranchStart: "Start",
          defaultBranchDone: "Done",
        }),
      ).toBe(true);
    });

    it("returns false if name was changed", () => {
      expect(
        isUntouchedBlankSeed({
          name: "Custom Name",
          description: "",
          steps: blankSeedSteps,
          defaultName: "New Workflow",
          defaultBranchStart: "Start",
          defaultBranchDone: "Done",
        }),
      ).toBe(false);
    });

    it("returns false if description was entered", () => {
      expect(
        isUntouchedBlankSeed({
          name: "New Workflow",
          description: "My description",
          steps: blankSeedSteps,
          defaultName: "New Workflow",
          defaultBranchStart: "Start",
          defaultBranchDone: "Done",
        }),
      ).toBe(false);
    });

    it("returns false if steps count is different", () => {
      expect(
        isUntouchedBlankSeed({
          name: "New Workflow",
          description: "",
          steps: blankSeedSteps.slice(0, 3),
          defaultName: "New Workflow",
          defaultBranchStart: "Start",
          defaultBranchDone: "Done",
        }),
      ).toBe(false);
    });

    it("returns false if instructions were added", () => {
      const modified = [
        { ...blankSeedSteps[0]!, instructions: "Check something" },
        blankSeedSteps[1]!,
        blankSeedSteps[2]!,
        blankSeedSteps[3]!,
      ];
      expect(
        isUntouchedBlankSeed({
          name: "New Workflow",
          description: "",
          steps: modified,
          defaultName: "New Workflow",
          defaultBranchStart: "Start",
          defaultBranchDone: "Done",
        }),
      ).toBe(false);
    });

    it("returns false if transitions were modified", () => {
      const modified = [
        { ...blankSeedSteps[0]!, transitions: [] },
        blankSeedSteps[1]!,
        blankSeedSteps[2]!,
        blankSeedSteps[3]!,
      ];
      expect(
        isUntouchedBlankSeed({
          name: "New Workflow",
          description: "",
          steps: modified,
          defaultName: "New Workflow",
          defaultBranchStart: "Start",
          defaultBranchDone: "Done",
        }),
      ).toBe(false);
    });
  });

  describe("slugifyStatusName", () => {
    it("converts status names into clean lowercase ASCII keys", () => {
      expect(slugifyStatusName("Analysis")).toBe("analysis");
      expect(slugifyStatusName("Stage done")).toBe("stage_done");
      expect(slugifyStatusName("Plan ready")).toBe("plan_ready");
      expect(slugifyStatusName("QA")).toBe("qa");
      expect(slugifyStatusName("Security review")).toBe("security_review");
    });
  });

  describe("deriveStepOrder", () => {
    it("orders nodes left-to-right on a single row", () => {
      const nodes = [
        { status_key: "done", layout: { x: 900, y: 60 } },
        { status_key: "todo", layout: { x: 60, y: 60 } },
        { status_key: "in_progress", layout: { x: 380, y: 70 } },
      ];
      const ordered = deriveStepOrder(nodes);
      expect(ordered.map((n) => n.status_key)).toEqual(["todo", "in_progress", "done"]);
    });

    it("orders nodes left-to-right then top-to-bottom across multiple rows", () => {
      const nodes = [
        { status_key: "step-6", layout: { x: 60, y: 280 } },
        { status_key: "step-1", layout: { x: 60, y: 60 } },
        { status_key: "step-7", layout: { x: 380, y: 280 } },
        { status_key: "step-2", layout: { x: 380, y: 60 } },
      ];
      const ordered = deriveStepOrder(nodes);
      expect(ordered.map((n) => n.status_key)).toEqual(["step-1", "step-2", "step-6", "step-7"]);
    });
  });

  describe("auto-layout helpers", () => {
    it("generates positions with at most 5 nodes per row", () => {
      const p0 = getAutoLayoutPosition(0);
      const p4 = getAutoLayoutPosition(4);
      const p5 = getAutoLayoutPosition(5);

      expect(p0.y).toBe(60);
      expect(p4.y).toBe(60);
      expect(p5.y).toBe(280); // Next row
      expect(p5.x).toBe(60); // Resets to first column
    });

    it("assigns layout positions to steps missing layout", () => {
      const steps = [
        { status_key: "todo" },
        { status_key: "in_progress", layout: { x: 100, y: 100 } },
        { status_key: "done" },
      ];
      const placed = ensureStepLayouts(steps);
      expect(placed[0]?.layout).toEqual(getAutoLayoutPosition(0));
      expect(placed[1]?.layout).toEqual({ x: 100, y: 100 });
      expect(placed[2]?.layout).toEqual(getAutoLayoutPosition(2));
    });
  });
});

describe("created status keys (matches the server's create path)", () => {
  const started = { category: "started" };

  it("caps a derived key at the server's 32 characters", () => {
    expect(slugifyStatusName("A very long status name that goes past thirty two characters")).toBe(
      "a_very_long_status_name_that_goe",
    );
  });

  it("reuses a live status with the same name, ignoring case", () => {
    const catalog = [{ key: "waiting_room", name: "Waiting" }];
    expect(resolveCreatedStatusKey({ name: "waiting", ...started }, catalog, new Set(["waiting_room"]))).toBe(
      "waiting_room",
    );
  });

  it("does not reuse an archived status, and suffixes a key it still holds", () => {
    const catalog = [{ key: "waiting", name: "Waiting", archived_at: "2026-11-01T00:00:00Z" }];
    expect(resolveCreatedStatusKey({ name: "Waiting", ...started }, catalog, new Set(["waiting"]))).toBe("waiting_2");
  });

  it("suffixes a key another status holds, and never a reserved built-in key", () => {
    expect(resolveCreatedStatusKey({ name: "Triage", ...started }, [{ key: "triage", name: "Old" }], new Set(["triage"]))).toBe(
      "triage_2",
    );
    expect(resolveCreatedStatusKey({ name: "Done", ...started }, [], new Set())).toBe("done_2");
  });

  it("keys a created column and points the transitions that name it at the same key", () => {
    const steps = cloneWorkflowSteps(
      [
        {
          status_key: "todo",
          handler: { type: "none" },
          instructions: "",
          transitions: [{ id: "t", label: "Default", when: null, to_status_key: "waiting" }],
        },
        {
          new_status: { name: "Waiting", category: "started" },
          handler: { type: "none" },
          instructions: "",
          transitions: [{ id: "w", label: "Default", when: null, to_status_key: "done" }],
        },
      ],
      [{ key: "waiting", name: "Waiting But Not Waiting", archived_at: null }],
    );
    expect(steps[1]?.status_key).toBe("waiting_2");
    expect(steps[0]?.transitions?.[0]?.to_status_key).toBe("waiting_2");
    expect(steps[1]?.new_status).toEqual({ name: "Waiting", category: "started" });
  });
});
