// @vitest-environment node
// Guards the text a Kit Viet agent reads about its sub-issue status and about blocked stops (S7 defects D2-D4).
// A stage barrier releases the next stage only when every sub-issue is done, so no instruction may tell a stage
// agent to set a sub-issue to in_review, and every blocked stop must mention the requester with the markup a
// person's notification needs. The texts are the generated skills and the composed role instructions.

import { describe, expect, it } from "vitest";
import { KV_ROLE_KINDS, composeKvAgentInstructions } from "./kitviet-agent-instructions";
import { KITVIET_BUILTIN_SKILLS } from "./kitviet-skills-data/common";

function skillContent(name: string): string {
  const skill = KITVIET_BUILTIN_SKILLS.find((s) => s.name === name);
  if (!skill) throw new Error(`missing skill ${name}`);
  return skill.content;
}

const DELIVERY = skillContent("kv--delivery-core");
const LEAD = skillContent("kv--squad-lead");
const REQUESTER_MENTION = "](mention://member/<requester id>)";

// A sentence that names in_review without saying it is the request's status or forbidden is an instruction
// to a stage agent to use it on a sub-issue.
function stageInReviewInstructions(text: string): string[] {
  return text
    .split(/(?<=[.!?])\s+|\n+/)
    .filter((s) => s.includes("in_review") && !/never|not\b|only|request/i.test(s));
}

const roleTexts = KV_ROLE_KINDS.map((kind) => [kind, composeKvAgentInstructions({ kind })] as const);

describe("stage sub-issues never end in_review (S7 D2, D3)", () => {
  it("the delivery core states the rule", () => {
    expect(DELIVERY).toContain("Never `in_review`");
  });

  it("the delivery core's stage output template offers done or blocked only", () => {
    expect(DELIVERY).toContain("Kết quả: <stage> <done | blocked>");
    expect(DELIVERY).not.toContain("in_review>");
  });

  it.each([
    ["kv--delivery-core", DELIVERY],
    ["kv--squad-lead", LEAD],
  ])("%s has no instruction that sets a sub-issue to in_review", (_name, text) => {
    expect(stageInReviewInstructions(text)).toEqual([]);
  });

  it.each(roleTexts)("the %s role text has no instruction that sets a sub-issue to in_review", (_kind, text) => {
    expect(stageInReviewInstructions(text)).toEqual([]);
  });
});

describe("blocked stops mention the requester (S7 D4)", () => {
  it("the delivery core gives the exact markup and where to read the requester id", () => {
    expect(DELIVERY).toContain(`[@<requester name>](mention://member/<requester id>)`);
    expect(DELIVERY).toContain("creator_id");
    expect(DELIVERY).toContain("creator_type");
  });

  it("every stop template of the lead carries the requester mention", () => {
    const stops = [...LEAD.matchAll(/^(BLOCKED ·|IN REVIEW ·)/gm)].map((m) => m.index ?? 0);
    expect(stops.length).toBe(3);
    for (const at of stops) {
      expect(LEAD.slice(at, at + 600)).toContain(REQUESTER_MENTION);
    }
  });

  it.each(roleTexts.filter(([kind]) => kind !== "kv_lead"))(
    "the %s role text tells the agent to mention the requester in a blocked comment",
    (_kind, text) => {
      expect(text).toContain(REQUESTER_MENTION);
    },
  );
});

describe("round counting stays with the lead (S7 run 1)", () => {
  it("the lead re-runs every gate after a fix and counts fix sub-issues itself", () => {
    expect(LEAD).toContain("create **every** gate of the plan again");
    expect(LEAD).toContain("two fix rounds");
    expect(LEAD).toContain("count rounds yourself");
  });

  it("a gate gives a verdict only and never decides a round or a stop", () => {
    const gate = composeKvAgentInstructions({ kind: "kv_gate" });
    expect(gate).toContain("Give a verdict only");
  });
});
