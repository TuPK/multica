import { describe, expect, it } from "vitest";
import { getConditionDepth, validateWorkflow } from "./validation";
import type { Condition, IssueWorkflowStep, IssueWorkflowTransition } from "./types";

describe("getConditionDepth", () => {
  it("returns 0 for null condition", () => {
    expect(getConditionDepth(null)).toBe(0);
  });

  it("returns 1 for a single rule", () => {
    expect(
      getConditionDepth({ field: "outcome", op: "eq", value: "approved" }),
    ).toBe(1);
  });

  it("calculates nested condition depth accurately", () => {
    const depth2: Condition = {
      all: [{ field: "outcome", op: "eq", value: "approved" }],
    };
    expect(getConditionDepth(depth2)).toBe(2);

    const depth3: Condition = {
      all: [
        {
          any: [{ field: "outcome", op: "eq", value: "approved" }],
        },
      ],
    };
    expect(getConditionDepth(depth3)).toBe(3);

    const depth5: Condition = {
      all: [
        {
          any: [
            {
              not: {
                all: [{ field: "priority", op: "eq", value: "urgent" }],
              },
            },
          ],
        },
      ],
    };
    expect(getConditionDepth(depth5)).toBe(5);
  });
});

describe("validateWorkflow", () => {
  const validSteps: IssueWorkflowStep[] = [
    {
      status_key: "todo",
      handler: { type: "none" },
      instructions: "",
      transitions: [{ id: "t1", label: "Start", when: null, to_status_key: "in_progress" }],
    },
    {
      status_key: "in_progress",
      handler: { type: "none" },
      instructions: "",
      transitions: [
        {
          id: "t2",
          label: "Approve",
          when: { field: "outcome", op: "eq", value: "approved" },
          to_status_key: "done",
        },
        {
          id: "t3",
          label: "Reject",
          when: null, // default fallback
          to_status_key: "cancelled",
        },
      ],
    },
    {
      status_key: "done",
      handler: { type: "none" },
      instructions: "",
    },
    {
      status_key: "cancelled",
      handler: { type: "none" },
      instructions: "",
    },
  ];

  it("passes a valid workflow with zero issues", () => {
    const issues = validateWorkflow(validSteps);
    expect(issues.filter((i) => i.severity === "error")).toHaveLength(0);
  });

  it("flags missing required statuses", () => {
    const stepsWithoutDone = validSteps.filter((s) => s.status_key !== "done");
    const issues = validateWorkflow(stepsWithoutDone);
    expect(issues.some((i) => i.id === "missing-done")).toBe(true);
  });

  it("flags fixed head ordering violation", () => {
    const stepsWithMisplacedBacklog: IssueWorkflowStep[] = [
      validSteps[0]!,
      { status_key: "backlog", handler: { type: "none" }, instructions: "" },
      ...validSteps.slice(1),
    ];
    const issues = validateWorkflow(stepsWithMisplacedBacklog);
    expect(issues.some((i) => i.id === "fixed-head-backlog")).toBe(true);
  });

  it("flags missing default branch when multiple branches exist", () => {
    const stepsWithNoDefault: IssueWorkflowStep[] = [
      validSteps[0]!,
      {
        status_key: "in_progress",
        handler: { type: "none" },
        instructions: "",
        transitions: [
          {
            id: "t2",
            label: "Approve",
            when: { field: "outcome", op: "eq", value: "approved" },
            to_status_key: "done",
          },
          {
            id: "t3",
            label: "Reject",
            when: { field: "outcome", op: "eq", value: "rejected" },
            to_status_key: "cancelled",
          },
        ],
      },
      validSteps[2]!,
      validSteps[3]!,
    ];
    const issues = validateWorkflow(stepsWithNoDefault);
    expect(issues.some((i) => i.id === "missing-default-in_progress")).toBe(true);
  });

  it("flags multiple default branches on one step", () => {
    const stepsWithTwoDefaults: IssueWorkflowStep[] = [
      validSteps[0]!,
      {
        status_key: "in_progress",
        handler: { type: "none" },
        instructions: "",
        transitions: [
          { id: "t2", label: "Default 1", when: null, to_status_key: "done" },
          { id: "t3", label: "Default 2", when: null, to_status_key: "cancelled" },
        ],
      },
      validSteps[2]!,
      validSteps[3]!,
    ];
    const issues = validateWorkflow(stepsWithTwoDefaults);
    expect(issues.some((i) => i.id === "multiple-defaults-in_progress")).toBe(true);
  });

  it("flags default branch when not last in order", () => {
    const stepsWithDefaultNotLast: IssueWorkflowStep[] = [
      validSteps[0]!,
      {
        status_key: "in_progress",
        handler: { type: "none" },
        instructions: "",
        transitions: [
          { id: "t2", label: "Default", when: null, to_status_key: "done" },
          {
            id: "t3",
            label: "Special",
            when: { field: "outcome", op: "eq", value: "urgent" },
            to_status_key: "cancelled",
          },
        ],
      },
      validSteps[2]!,
      validSteps[3]!,
    ];
    const issues = validateWorkflow(stepsWithDefaultNotLast);
    expect(issues.some((i) => i.id === "default-not-last-in_progress")).toBe(true);
  });

  it("flags unknown transition target status", () => {
    const stepsWithBadTarget: IssueWorkflowStep[] = [
      {
        ...validSteps[0]!,
        transitions: [{ id: "t1", label: "Go", when: null, to_status_key: "non_existent" }],
      },
      ...validSteps.slice(1),
    ];
    const issues = validateWorkflow(stepsWithBadTarget);
    expect(issues.some((i) => i.id === "invalid-target-t1")).toBe(true);
  });

  it("flags condition depth exceeding 4", () => {
    const deepCondition: Condition = {
      all: [
        {
          any: [
            {
              not: {
                all: [{ field: "outcome", op: "eq", value: "done" }],
              },
            },
          ],
        },
      ],
    };
    const stepsWithDeepCond: IssueWorkflowStep[] = [
      {
        ...validSteps[0]!,
        transitions: [{ id: "t1", label: "Go", when: deepCondition, to_status_key: "done" }],
      },
      ...validSteps.slice(1),
    ];
    const issues = validateWorkflow(stepsWithDeepCond);
    expect(issues.some((i) => i.id === "condition-depth-t1")).toBe(true);
  });

  it("flags cycle that never reaches terminal status with a warning", () => {
    const cycleSteps: IssueWorkflowStep[] = [
      {
        status_key: "todo",
        handler: { type: "none" },
        instructions: "",
        transitions: [{ id: "t1", label: "Loop A", when: null, to_status_key: "step_b" }],
      },
      {
        status_key: "step_b",
        handler: { type: "none" },
        instructions: "",
        transitions: [{ id: "t2", label: "Loop Back", when: null, to_status_key: "todo" }],
      },
      { status_key: "done", handler: { type: "none" }, instructions: "" },
      { status_key: "cancelled", handler: { type: "none" }, instructions: "" },
    ];
    const issues = validateWorkflow(cycleSteps);
    const warnings = issues.filter((i) => i.severity === "warning");
    expect(warnings.length).toBeGreaterThan(0);
    expect(warnings.some((w) => w.step_key === "todo" || w.step_key === "step_b")).toBe(true);
  });
});

describe("validateWorkflow against the server's transition rules", () => {
  const branch = (overrides: Partial<IssueWorkflowTransition>): IssueWorkflowTransition => ({
    id: "t",
    label: "Go",
    when: null,
    to_status_key: "done",
    ...overrides,
  });
  const workflowWith = (transitions: IssueWorkflowTransition[], handler: IssueWorkflowStep["handler"] = { type: "none" }) => [
    { status_key: "todo", handler: { type: "none" }, instructions: "", transitions: [branch({ id: "start", label: "Start", to_status_key: "in_progress" })] },
    { status_key: "in_progress", handler, instructions: "", transitions },
    { status_key: "done", handler: { type: "none" }, instructions: "" },
    { status_key: "cancelled", handler: { type: "none" }, instructions: "" },
  ] as IssueWorkflowStep[];
  const errorIds = (steps: IssueWorkflowStep[]) =>
    validateWorkflow(steps)
      .filter((i) => i.severity === "error")
      .map((i) => i.id);

  it("flags a branch with an empty label, which the server refuses", () => {
    expect(errorIds(workflowWith([branch({ id: "a", label: "  " })]))).toContain("label-required-a");
  });

  it("flags a branch that routes its step to itself", () => {
    expect(errorIds(workflowWith([branch({ id: "a", to_status_key: "in_progress" })]))).toContain(
      "self-target-a",
    );
  });

  it("flags two branches sharing one id", () => {
    const ids = errorIds(workflowWith([branch({ id: "a" }), branch({ id: "a", when: null })]));
    expect(ids).toContain("duplicate-transition-a");
  });

  it("flags an agent or member handler with no id to hand off to", () => {
    expect(errorIds(workflowWith([branch({ id: "a" })], { type: "agent" }))).toContain("handler-id-in_progress");
    expect(errorIds(workflowWith([branch({ id: "a" })], { type: "member" }))).toContain("handler-id-in_progress");
    expect(errorIds(workflowWith([branch({ id: "a" })], { type: "agent", id: "agent-1" }))).toEqual([]);
  });

  it("flags a handler type the server does not hand off to", () => {
    const squad = { type: "squad", id: "sq-1" } as IssueWorkflowStep["handler"];
    expect(errorIds(workflowWith([branch({ id: "a" })], squad))).toContain("handler-type-in_progress");
  });

  it("reports each invalid condition on its branch", () => {
    const badRule = branch({
      id: "a",
      when: { field: "outcome", op: "gt", value: "approved" },
    });
    const issues = validateWorkflow(workflowWith([badRule, branch({ id: "b" })]));
    const conditionIssue = issues.find((i) => i.transition_id === "a" && i.severity === "error");
    expect(conditionIssue?.message).toEqual({
      key: "condition_operator_not_for_field",
      values: { label: "Go", op: "gt", field: "outcome" },
    });
  });

  it("passes a branch whose condition the server accepts", () => {
    const routed = branch({
      id: "a",
      when: { all: [{ field: "rework_count", op: "gte", value: 3 }, { field: "labels", op: "exists" }] },
    });
    expect(errorIds(workflowWith([routed, branch({ id: "b" })]))).toEqual([]);
  });
});
