// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  GATE_STEP_TEMPLATE,
  HUMAN_APPROVAL_TEMPLATE,
  QUORUM_REVIEW_TEMPLATE,
} from "./step-templates";

describe("step-templates", () => {
  it("Gate step template generates the four standard edges and N is a valid count", () => {
    const gate = GATE_STEP_TEMPLATE.createStep({
      key: "review_gate",
      title: "Review Gate",
      nextStepKey: "deploy",
      loopHeaderKey: "dev",
      loopKey: "fix_loop",
      stopCode: "missing_grounds",
    });

    expect(gate.kind).toBe("tasks");
    expect(gate.edges).toHaveLength(4);

    // 1. Block -> stop
    expect(gate.edges[0]).toMatchObject({
      label: "Blocked",
      stop: "missing_grounds",
      when: { field: "outcome", op: "eq", value: "blocked" },
    });

    // 2. Pass -> next
    expect(gate.edges[1]).toMatchObject({
      label: "Passed",
      to: "deploy",
      when: { field: "outcome", op: "eq", value: "approved" },
    });

    // 3. Fail + loop -> back to header
    expect(gate.edges[2]).toMatchObject({
      label: "Rework",
      to: "dev",
      loop: "fix_loop",
      when: { field: "outcome", op: "eq", value: "rejected" },
    });

    // 4. Default -> stop
    expect(gate.edges[3]).toMatchObject({
      label: "Default stop",
      stop: "stalled",
    });
  });

  it("the Quorum review template generates a quorum join and the review verdict set", () => {
    const quorum = QUORUM_REVIEW_TEMPLATE.createStep({
      key: "quorum_review",
      title: "Quorum Review",
      nextStepKey: "merge",
      quorumK: 3,
    });

    expect(quorum.kind).toBe("tasks");
    expect(quorum.join).toEqual({
      over: "gates",
      mode: "quorum",
      k: 3,
      block: "immediate",
    });
    expect(quorum.tasks?.[0]?.verdicts).toBe("review");
    expect(quorum.tasks?.[1]?.verdicts).toBe("review");
  });

  it("the Human approval template generates a manual step and its notice", () => {
    const human = HUMAN_APPROVAL_TEMPLATE.createStep({
      key: "human_gate",
      title: "Human Approval",
      nextStepKey: "release",
    });

    expect(human.kind).toBe("manual");
    expect(human.exit_role).toBe("managers");
    expect(human.notice).toBe("human_approval_required");
    expect(human.edges[0]?.trigger).toBe("manual");
  });
});
