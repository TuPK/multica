// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { FlowDoc } from "@multica/core/types";
import { addStepFromTemplate, STEP_TEMPLATE_IDS, type StepTemplateCopy } from "./step-templates";

const copy: StepTemplateCopy = {
  stepTitle: { work: "Work", gate: "Gate", quorum: "Quorum review", human: "Human approval", lead: "Lead decision" },
  taskWork: "Do the work",
  taskGate: "Check the work",
  taskReviewer: (n) => `Reviewer ${n}`,
  edgeNext: "Next",
  edgeBlocked: "Blocked",
  edgePassed: "Passed",
  edgeRework: "Send back",
  edgeStopped: "Stopped",
  edgeRelease: "Approve and continue",
  noticeHumanApproval: "{requester} {request} is waiting for your approval.",
};

function baseDoc(extra: Partial<FlowDoc> = {}): FlowDoc {
  return {
    version: 2,
    start: "make",
    terminals: [{ key: "done", status: "done" }],
    roles: [{ key: "maker", kind: "agent" }],
    steps: [
      {
        key: "make",
        title: "Make",
        status: { key: "in_progress" },
        kind: "tasks",
        tasks: [{ key: "make_it", title: "Make it", kind: "work", role: "maker" }],
        edges: [{ id: "make_next", label: "Next", to: "done" }],
      },
    ],
    ...extra,
  };
}

describe("step templates", () => {
  it("add one step each and leave the rest of the document alone", () => {
    for (const id of STEP_TEMPLATE_IDS) {
      const before = baseDoc();
      const snapshot = JSON.stringify(before);
      const { doc, stepKey } = addStepFromTemplate(before, id, copy);
      expect(JSON.stringify(before), `${id} changed its input`).toBe(snapshot);
      expect(doc.steps).toHaveLength(2);
      expect(doc.steps[0]).toEqual(before.steps[0]);
      const added = doc.steps[1]!;
      expect(added.key).toBe(stepKey);
      expect(added.title).toBe(copy.stepTitle[id]);
      expect(added.status?.key, `${id} shows a status that every workspace has`).toBeTruthy();
      // Every edge leaves by an id of its own and goes to a terminal, ready for a person to re-point.
      const ids = added.edges.map((e) => e.id);
      expect(new Set(ids).size).toBe(ids.length);
      for (const edge of added.edges) expect(edge.to ?? edge.stop).toBeTruthy();
    }
  });

  it("Gate has the four standard edges when the document has a loop to go back along", () => {
    const { doc, stepKey } = addStepFromTemplate(
      baseDoc({ loops: [{ key: "fix", header: "make", max: 2 }] }),
      "gate",
      copy,
    );
    const gate = doc.steps.find((s) => s.key === stepKey)!;
    expect(gate.kind).toBe("tasks");
    expect(gate.tasks).toHaveLength(1);
    expect(gate.tasks![0]).toMatchObject({ kind: "gate", title: copy.taskGate });
    expect(gate.join).toEqual({ over: "gates", mode: "all", block: "immediate" });
    expect(gate.edges.map((e) => e.label)).toEqual(["Blocked", "Passed", "Send back", "Stopped"]);
    const [blocked, passed, rework, stopped] = gate.edges;
    expect(blocked).toMatchObject({ stop: "stalled", when: { field: "join.result", op: "eq", value: "block" } });
    expect(passed).toMatchObject({ to: "done", when: { field: "join.result", op: "eq", value: "pass" } });
    expect(rework).toMatchObject({
      to: "make",
      loop: "fix",
      when: {
        all: [
          { field: "join.result", op: "eq", value: "fail" },
          { field: "loop.fix.count", op: "lt", value: 2 },
        ],
      },
    });
    expect(stopped).toEqual({ id: `${stepKey}_stopped`, label: "Stopped", stop: "stalled" });
  });

  it("Gate has no way back when there is no loop, and a loop bound by a variable is not used", () => {
    const none = addStepFromTemplate(baseDoc(), "gate", copy);
    expect(none.doc.steps[1]!.edges.map((e) => e.label)).toEqual(["Blocked", "Passed", "Stopped"]);
    const fromVar = addStepFromTemplate(baseDoc({ loops: [{ key: "fix", header: "make", max: { var: "rounds", cap: 5 } }] }), "gate", copy);
    expect(fromVar.doc.steps[1]!.edges.map((e) => e.label)).toEqual(["Blocked", "Passed", "Stopped"]);
  });

  it("a gate is judged by a role that does no work, and a new role is added when every agent works", () => {
    const withReviewer = addStepFromTemplate(baseDoc({ roles: [{ key: "maker", kind: "agent" }, { key: "checker", kind: "agent" }] }), "gate", copy);
    expect(withReviewer.doc.steps[1]!.tasks![0]!.role).toBe("checker");
    expect(withReviewer.doc.roles).toHaveLength(2);

    const alone = addStepFromTemplate(baseDoc(), "gate", copy);
    expect(alone.doc.steps[1]!.tasks![0]!.role).toBe("reviewer");
    expect(alone.doc.roles).toEqual([{ key: "maker", kind: "agent" }, { key: "reviewer", kind: "agent" }]);
  });

  it("Quorum review asks three reviewers and passes when two approve, with a pool to match", () => {
    const { doc, stepKey } = addStepFromTemplate(baseDoc(), "quorum", copy);
    const step = doc.steps.find((s) => s.key === stepKey)!;
    expect(step.join).toEqual({ over: "gates", mode: "quorum", k: 2, block: "immediate" });
    expect(step.tasks!.map((t) => t.title)).toEqual(["Reviewer 1", "Reviewer 2", "Reviewer 3"]);
    expect(new Set(step.tasks!.map((t) => t.key)).size).toBe(3);
    const role = doc.roles!.find((r) => r.key === step.tasks![0]!.role)!;
    expect(role).toMatchObject({ kind: "agent", pool: 3 });
    expect(step.tasks!.every((t) => t.role === role.key)).toBe(true);
  });

  it("Human approval is a manual step for the managers with a notice of its own", () => {
    const { doc, stepKey } = addStepFromTemplate(baseDoc({ messages: { human_approval: "mine" } }), "human", copy);
    const step = doc.steps.find((s) => s.key === stepKey)!;
    expect(step).toMatchObject({ kind: "manual", exit_role: "managers" });
    expect(step.edges).toEqual([{ id: `${stepKey}_release`, label: "Approve and continue", to: "done", trigger: "manual" }]);
    // The notice takes a free key and leaves the message that was there.
    expect(step.notice).toBe("human_approval_1");
    expect(doc.messages).toEqual({ human_approval: "mine", human_approval_1: copy.noticeHumanApproval });
  });

  it("Work and Lead decision add one task for an agent, or none for the lead", () => {
    const work = addStepFromTemplate(baseDoc(), "work", copy);
    expect(work.doc.steps[1]!.tasks).toEqual([{ key: `${work.stepKey}_do`, title: "Do the work", kind: "work", role: "maker" }]);
    const lead = addStepFromTemplate(baseDoc(), "lead", copy);
    expect(lead.doc.steps[1]).toMatchObject({ kind: "lead", lead: {} });
    expect(lead.doc.steps[1]!.tasks).toBeUndefined();
  });

  it("a work step on a document without an agent role adds one", () => {
    const { doc } = addStepFromTemplate(baseDoc({ roles: [] }), "work", copy);
    expect(doc.roles).toEqual([{ key: "worker", kind: "agent" }]);
    expect(doc.steps[1]!.tasks![0]!.role).toBe("worker");
  });

  it("takes a step key that is free, also after a step was deleted", () => {
    const first = addStepFromTemplate(baseDoc(), "gate", copy);
    const second = addStepFromTemplate(first.doc, "gate", copy);
    expect([first.stepKey, second.stepKey]).toEqual(["gate_1", "gate_2"]);
    const gaps = addStepFromTemplate({ ...second.doc, steps: second.doc.steps.filter((s) => s.key !== "gate_1") }, "gate", copy);
    expect(gaps.stepKey).toBe("gate_1");
  });

  it("leaves an edge without a target when the document has no terminal", () => {
    const { doc } = addStepFromTemplate(baseDoc({ terminals: [] }), "work", copy);
    expect(doc.steps[1]!.edges[0]).toEqual({ id: `${doc.steps[1]!.key}_next`, label: "Next" });
  });
});
