// @vitest-environment node
// Guards the texts a Kit Viet lead reads now that a workflow flow can run its request (spec section 13, D3, D9):
// the lead looks at the flow first, keeps the old procedure for a request no flow runs, and no text tells the lead
// to rerun only the gate that blocked. Wording only; no lead is run here.

import { describe, expect, it } from "vitest";
import { KITVIET_BUILTIN_SKILLS } from "../catalog/generated/kitviet/common";
import { KITVIET_SQUADS } from "../catalog/generated/kitviet/squads";
import { KV_LEAD_NOTES, composeKvAgentInstructions } from "./kitviet-agent-instructions";
import { KITVIET_PREVIOUS_AGENT_INSTRUCTIONS, KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS } from "./kitviet-previous-texts";

const LEAD_SKILL = KITVIET_BUILTIN_SKILLS.find((s) => s.name === "kv--squad-lead")!.content;
const LEAD_KIND = composeKvAgentInstructions({ kind: "kv_lead" });
const FLOW_SHOW = "multica issue flow show";

describe("the lead reads the flow first", () => {
  it("starts its instructions with the flow check and keeps the old procedure", () => {
    expect(LEAD_KIND.startsWith(`You are`)).toBe(true);
    const body = LEAD_KIND.slice(LEAD_KIND.indexOf("\n\n") + 2);
    expect(body.startsWith(`First, run \`${FLOW_SHOW} <request id>\``)).toBe(true);
    expect(body).toContain('{"managed": false}');
    expect(body).toContain("follow the procedure below and in the kv--squad-lead skill");
    expect(body).toContain("Your job: turn a request into sub-issues");
  });

  it("tells the lead, in the skill, how to work in each mode", () => {
    const mode = LEAD_SKILL.slice(LEAD_SKILL.indexOf("## Which mode"), LEAD_SKILL.indexOf("## Never"));
    expect(mode).toContain(`\`${FLOW_SHOW} <request id>\``);
    expect(mode).toContain("multica issue flow plan <request id>");
    expect(mode).toContain("multica issue flow advance <request id>");
    expect(mode).toContain("--blocked");
    expect(mode).toContain('{"managed": false}');
    expect(mode).toContain("Follow the steps below as written");
  });

  it("keeps the three stop headers where the lead skill had them", () => {
    for (const header of ["BLOCKED · stop 1 · thiếu căn cứ", "IN REVIEW · stop 2 · trước khi ra khỏi workspace", "BLOCKED · stop 3 · hết 2 vòng sửa"]) {
      expect(LEAD_SKILL).toContain(`\n${header}\n`);
    }
  });
});

describe("a fix reruns every gate (D3)", () => {
  it("no note or skill says the lead reruns only the gate that blocked", () => {
    const texts = [LEAD_SKILL, LEAD_KIND, ...Object.values(KV_LEAD_NOTES).filter((t): t is string => t !== undefined)];
    for (const text of texts) expect(text).not.toMatch(/reruns only the gate/i);
  });

  it("the lead skill and the lead instructions both say every gate runs again", () => {
    expect(LEAD_SKILL).toMatch(/create \*\*every\*\* gate of the plan again/);
    expect(LEAD_KIND).toContain("create every gate of the plan again, not only the gate that blocked");
  });

  it("the three notes that said otherwise now say the lead reruns every gate of the plan", () => {
    for (const squad of ["kv_officeops", "kv_education", "kv_teaching"]) {
      expect(KV_LEAD_NOTES[squad], squad).toContain("the lead reruns every gate of the plan, not only the gate that blocked");
    }
  });
});

describe("agents and squads made before this change can still be upgraded", () => {
  const leads = KITVIET_SQUADS.flatMap((squad) => squad.roles.filter((role) => role.kind === "kv_lead").map((role) => role.key));
  const beforeFlow = (text: string) => text.includes("Your job: turn a request into sub-issues") && !text.includes(FLOW_SHOW);

  it("lists the earlier lead text of every squad", () => {
    expect(leads).toHaveLength(21);
    for (const key of leads) expect((KITVIET_PREVIOUS_AGENT_INSTRUCTIONS[key] ?? []).some(beforeFlow), key).toBe(true);
  });

  it("lists the earlier squad instructions, one per squad", () => {
    expect(KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS.filter(beforeFlow).length).toBeGreaterThanOrEqual(leads.length);
  });
});
