import type { FlowDocEdge, FlowDocJoin, FlowDocStep, FlowDocTask } from "@multica/core/types";

export interface StepTemplate {
  name: string;
  description: string;
  createStep: (params: {
    key: string;
    title: string;
    nextStepKey?: string;
    loopHeaderKey?: string;
    loopKey?: string;
    stopCode?: string;
    quorumK?: number;
    roles?: string[];
  }) => FlowDocStep;
}

export const WORK_STEP_TEMPLATE: StepTemplate = {
  name: "Work",
  description: "One or more tasks performed by squad members or agents.",
  createStep: ({ key, title, nextStepKey, roles = [] }) => {
    const roleKey = roles[0] ?? "worker";
    const task: FlowDocTask = {
      key: `${key}_task`,
      title: `${title} Task`,
      kind: "work",
      role: roleKey,
    };
    const edges: FlowDocEdge[] = nextStepKey
      ? [
          {
            id: `${key}_to_${nextStepKey}`,
            label: "Next",
            to: nextStepKey,
            trigger: "auto",
          },
        ]
      : [];
    return {
      key,
      title,
      kind: "tasks",
      tasks: [task],
      join: { over: "all", mode: "all", block: "immediate" },
      edges,
    };
  },
};

export const GATE_STEP_TEMPLATE: StepTemplate = {
  name: "Gate",
  description: "A quality check gate with standard 4-edge routing: block, pass, fail (rework), default.",
  createStep: ({ key, title, nextStepKey, loopHeaderKey, loopKey, stopCode = "missing_grounds", roles = [] }) => {
    const roleKey = roles[0] ?? "reviewer";
    const task: FlowDocTask = {
      key: `${key}_check`,
      title: `${title} Check`,
      kind: "gate",
      role: roleKey,
      verdicts: "gate",
    };

    const edges: FlowDocEdge[] = [
      // 1. Block -> stop
      {
        id: `${key}_block`,
        label: "Blocked",
        stop: stopCode,
        when: { field: "outcome", op: "eq", value: "blocked" },
        trigger: "auto",
      },
      // 2. Pass -> next
      ...(nextStepKey
        ? [
            {
              id: `${key}_pass`,
              label: "Passed",
              to: nextStepKey,
              when: { field: "outcome", op: "eq", value: "approved" },
              trigger: "auto" as const,
            },
          ]
        : []),
      // 3. Fail + loop -> back to header
      ...(loopHeaderKey && loopKey
        ? [
            {
              id: `${key}_rework`,
              label: "Rework",
              to: loopHeaderKey,
              loop: loopKey,
              when: { field: "outcome", op: "eq", value: "rejected" },
              trigger: "auto" as const,
            },
          ]
        : []),
      // 4. Default -> stop
      {
        id: `${key}_default`,
        label: "Default stop",
        stop: "stalled",
        trigger: "auto",
      },
    ];

    return {
      key,
      title,
      kind: "tasks",
      tasks: [task],
      join: { over: "gates", mode: "all", block: "immediate" },
      edges,
    };
  },
};

export const QUORUM_REVIEW_TEMPLATE: StepTemplate = {
  name: "Quorum review",
  description: "Multiple reviewers with quorum approval threshold.",
  createStep: ({ key, title, nextStepKey, quorumK = 2, roles = [] }) => {
    const roleKey = roles[0] ?? "reviewer";
    const join: FlowDocJoin = {
      over: "gates",
      mode: "quorum",
      k: quorumK,
      block: "immediate",
    };
    const edges: FlowDocEdge[] = nextStepKey
      ? [
          {
            id: `${key}_approved`,
            label: "Approved",
            to: nextStepKey,
            when: { field: "join.result", op: "eq", value: "pass" },
            trigger: "auto",
          },
          {
            id: `${key}_rejected`,
            label: "Rejected",
            stop: "stalled",
            trigger: "auto",
          },
        ]
      : [];

    return {
      key,
      title,
      kind: "tasks",
      join,
      tasks: [
        {
          key: `${key}_r1`,
          title: "Reviewer 1",
          kind: "gate",
          role: roleKey,
          verdicts: "review",
        },
        {
          key: `${key}_r2`,
          title: "Reviewer 2",
          kind: "gate",
          role: roleKey,
          verdicts: "review",
        },
      ],
      edges,
    };
  },
};

export const HUMAN_APPROVAL_TEMPLATE: StepTemplate = {
  name: "Human approval",
  description: "A manual exit step requiring a person's decision.",
  createStep: ({ key, title, nextStepKey }) => {
    const edges: FlowDocEdge[] = [
      ...(nextStepKey
        ? [
            {
              id: `${key}_release`,
              label: "Approve & continue",
              to: nextStepKey,
              trigger: "manual" as const,
            },
          ]
        : []),
      {
        id: `${key}_stop`,
        label: "Stop",
        stop: "stalled",
        trigger: "manual" as const,
      },
    ];

    return {
      key,
      title,
      kind: "manual",
      exit_role: "managers",
      notice: "human_approval_required",
      edges,
    };
  },
};

export const LEAD_DECISION_TEMPLATE: StepTemplate = {
  name: "Lead decision",
  description: "Step planned and decided by the squad leader.",
  createStep: ({ key, title, nextStepKey }) => {
    return {
      key,
      title,
      kind: "lead",
      lead: {
        sets: [],
        verdicts: "plan",
      },
      edges: nextStepKey
        ? [
            {
              id: `${key}_next`,
              label: "Next",
              to: nextStepKey,
              trigger: "auto",
            },
          ]
        : [],
    };
  },
};

export const STEP_TEMPLATES: Record<string, StepTemplate> = {
  work: WORK_STEP_TEMPLATE,
  gate: GATE_STEP_TEMPLATE,
  quorum: QUORUM_REVIEW_TEMPLATE,
  human: HUMAN_APPROVAL_TEMPLATE,
  lead: LEAD_DECISION_TEMPLATE,
};
