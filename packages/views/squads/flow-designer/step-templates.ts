import type { FlowDoc, FlowDocEdge, FlowDocRole, FlowDocStep, FlowDocTask } from "@multica/core/types";

/**
 * Templates for adding a step (spec 11.x, "Mẫu bước"). Each one makes a step with the standard
 * edges of its kind, so that a person edits an edge instead of building the routing from nothing.
 * The step is added without any link into it: the person draws the edge that leads here.
 */
export type StepTemplateId = "work" | "gate" | "quorum" | "human" | "lead";

export const STEP_TEMPLATE_IDS: readonly StepTemplateId[] = ["work", "gate", "quorum", "human", "lead"];

/** The text the templates write into a document, in the language of the person who adds the step. */
export interface StepTemplateCopy {
  stepTitle: Record<StepTemplateId, string>;
  taskWork: string;
  taskGate: string;
  taskReviewer: (n: number) => string;
  edgeNext: string;
  edgeBlocked: string;
  edgePassed: string;
  edgeRework: string;
  edgeStopped: string;
  edgeRelease: string;
  /** Posted on the request when it reaches a human approval; it may use {requester} and {request}. */
  noticeHumanApproval: string;
}

/** Reviewers of the quorum template, and how many of them have to approve. */
const QUORUM_REVIEWERS = 3;
const QUORUM_K = 2;

// A status every workspace has; templates do not create statuses.
const WORK_STATUS = { key: "in_progress" };
const REVIEW_STATUS = { key: "in_review" };
const LEAD_STATUS = { key: "todo" };

function freeKey(taken: Iterable<string>, base: string): string {
  const used = new Set(taken);
  let n = 1;
  while (used.has(`${base}_${n}`)) n += 1;
  return `${base}_${n}`;
}

/** The first role an agent fills, or a new one when the document has none. */
function agentRole(roles: FlowDocRole[], base: string): { roles: FlowDocRole[]; key: string } {
  const existing = roles.find((r) => r.kind === "agent");
  if (existing) return { roles, key: existing.key };
  return addRole(roles, { key: base, kind: "agent" });
}

/**
 * The role that judges: an agent role no work task uses, because a gate has to be passed by someone
 * other than the person who did the work. A new role when every agent role works.
 */
function reviewerRole(doc: FlowDoc, roles: FlowDocRole[], pool?: number): { roles: FlowDocRole[]; key: string } {
  const working = new Set(doc.steps.flatMap((s) => (s.tasks ?? []).filter((t) => t.kind !== "gate").map((t) => t.role)));
  const free = pool ? undefined : roles.find((r) => r.kind === "agent" && !working.has(r.key));
  if (free) return { roles, key: free.key };
  return addRole(roles, pool ? { key: "reviewers", kind: "agent", pool } : { key: "reviewer", kind: "agent" });
}

function addRole(roles: FlowDocRole[], role: FlowDocRole): { roles: FlowDocRole[]; key: string } {
  const key = roles.some((r) => r.key === role.key) ? freeKey(roles.map((r) => r.key), role.key) : role.key;
  return { roles: [...roles, { ...role, key }], key };
}

/** A loop a rejected gate can send the request back along: one with a fixed bound that starts elsewhere. */
function reworkLoop(doc: FlowDoc, stepKey: string) {
  return (doc.loops ?? []).find((l) => typeof l.max === "number" && l.header !== stepKey);
}

/**
 * Adds a step made from a template to the end of the document. Returns the new document and the
 * key of the step. Edges that leave the step go to the first terminal of the document; the person
 * re-points them where the flow should go.
 */
export function addStepFromTemplate(
  doc: FlowDoc,
  id: StepTemplateId,
  copy: StepTemplateCopy,
): { doc: FlowDoc; stepKey: string } {
  const stepKey = freeKey(doc.steps.map((s) => s.key), id);
  const exit = doc.terminals[0]?.key;
  const title = copy.stepTitle[id];
  let roles = doc.roles ?? [];
  let step: FlowDocStep;
  let extraMessages: Record<string, string> | undefined;

  const goTo = (edge: Omit<FlowDocEdge, "to">): FlowDocEdge => (exit ? { ...edge, to: exit } : edge);

  switch (id) {
    case "work": {
      const role = agentRole(roles, "worker");
      roles = role.roles;
      const task: FlowDocTask = { key: `${stepKey}_do`, title: copy.taskWork, kind: "work", role: role.key };
      step = {
        key: stepKey,
        title,
        status: WORK_STATUS,
        kind: "tasks",
        tasks: [task],
        edges: [goTo({ id: `${stepKey}_next`, label: copy.edgeNext })],
      };
      break;
    }
    case "gate":
    case "quorum": {
      // The reviewers of a quorum are different people, so their role needs a pool of that size.
      const role = reviewerRole(doc, roles, id === "quorum" ? QUORUM_REVIEWERS : undefined);
      roles = role.roles;
      const count = id === "quorum" ? QUORUM_REVIEWERS : 1;
      const tasks: FlowDocTask[] = Array.from({ length: count }, (_, i) => ({
        key: `${stepKey}_check${count > 1 ? `_${i + 1}` : ""}`,
        title: count > 1 ? copy.taskReviewer(i + 1) : copy.taskGate,
        kind: "gate",
        role: role.key,
      }));
      const loop = reworkLoop(doc, stepKey);
      const edges: FlowDocEdge[] = [
        {
          id: `${stepKey}_blocked`,
          label: copy.edgeBlocked,
          when: { field: "join.result", op: "eq", value: "block" },
          stop: "stalled",
        },
        goTo({
          id: `${stepKey}_passed`,
          label: copy.edgePassed,
          when: { field: "join.result", op: "eq", value: "pass" },
        }),
        ...(loop && typeof loop.max === "number"
          ? [
              {
                id: `${stepKey}_rework`,
                label: copy.edgeRework,
                to: loop.header,
                loop: loop.key,
                when: {
                  all: [
                    { field: "join.result", op: "eq", value: "fail" },
                    { field: `loop.${loop.key}.count`, op: "lt", value: loop.max },
                  ],
                },
              } satisfies FlowDocEdge,
            ]
          : []),
        { id: `${stepKey}_stopped`, label: copy.edgeStopped, stop: "stalled" },
      ];
      step = {
        key: stepKey,
        title,
        status: REVIEW_STATUS,
        kind: "tasks",
        tasks,
        join:
          id === "quorum"
            ? { over: "gates", mode: "quorum", k: QUORUM_K, block: "immediate" }
            : { over: "gates", mode: "all", block: "immediate" },
        edges,
      };
      break;
    }
    case "human": {
      const messages = doc.messages ?? {};
      const notice = messages.human_approval === undefined ? "human_approval" : freeKey(Object.keys(messages), "human_approval");
      extraMessages = { [notice]: copy.noticeHumanApproval };
      step = {
        key: stepKey,
        title,
        status: REVIEW_STATUS,
        kind: "manual",
        exit_role: "managers",
        notice,
        edges: [goTo({ id: `${stepKey}_release`, label: copy.edgeRelease, trigger: "manual" })],
      };
      break;
    }
    case "lead": {
      step = {
        key: stepKey,
        title,
        status: LEAD_STATUS,
        kind: "lead",
        lead: {},
        edges: [goTo({ id: `${stepKey}_next`, label: copy.edgeNext })],
      };
      break;
    }
  }

  return {
    doc: {
      ...doc,
      roles,
      ...(extraMessages ? { messages: { ...doc.messages, ...extraMessages } } : {}),
      steps: [...doc.steps, step],
    },
    stepKey,
  };
}
