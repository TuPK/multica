// @vitest-environment node
import { describe, expect, it } from "vitest";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import { SALES_ROLE_AGENT_INSTRUCTIONS, type SalesRoleKey } from "./sales-agent-instructions";
import { SALES_SKILL_NAMES, SDLC_SKILL_NAMES, SQUAD_TEMPLATES } from "./templates";

const skill = (name: string) => {
  const found = SDLC_BUILTIN_SKILLS.find((s) => s.name === name);
  if (!found) throw new Error(`no built-in skill ${name}`);
  return found;
};

const bodyOf = (name: string) => [skill(name).content, ...skill(name).files.map((f) => f.content)].join("\n");

describe("sales skills", () => {
  it("ships the seven skills the sales squad names, each with a description and a procedure", () => {
    expect(SALES_SKILL_NAMES).toEqual([
      "sales-delivery-core",
      "sales-squad-lead",
      "sales-scouting",
      "sales-source-builder",
      "sales-writing",
      "sales-deal-analysis",
      "sales-review",
    ]);
    for (const name of SALES_SKILL_NAMES) {
      expect(skill(name).description.length, name).toBeGreaterThan(40);
      expect(skill(name).content.length, name).toBeGreaterThan(2000);
    }
  });

  it("keeps the sales skills out of every SDLC squad", () => {
    for (const name of SALES_SKILL_NAMES) expect(SDLC_SKILL_NAMES, name).not.toContain(name);
    for (const name of SDLC_SKILL_NAMES) expect(name.startsWith("sales-"), name).toBe(false);
  });

  it("gives the sales squad the sales skills and no other", () => {
    const sales = SQUAD_TEMPLATES.find((t) => t.id === "sales_loop")!;
    expect(sales.builtinSkills).toEqual(SALES_SKILL_NAMES);
  });

  it("makes every role load the core rules and its own skill, and names them in its instructions", () => {
    const ownSkill: Record<SalesRoleKey, string> = {
      sales_lead: "sales-squad-lead",
      sales_scout: "sales-scouting",
      sales_writer: "sales-writing",
      sales_analyst: "sales-deal-analysis",
      sales_checker: "sales-review",
    };
    for (const [role, name] of Object.entries(ownSkill) as [SalesRoleKey, string][]) {
      const text = SALES_ROLE_AGENT_INSTRUCTIONS[role];
      expect(text, role).toContain("sales-delivery-core");
      expect(text, role).toContain(name);
      expect(text, role).toContain("Never send");
    }
  });

  it("states the hard rules in the core skill: the base, dedupe keys, and the no-send rule", () => {
    const core = bodyOf("sales-delivery-core");
    expect(core).toContain('base `sales`');
    expect(core).toContain("sales:<kind>:<lane>:<entity_key>");
    expect(core).toContain("Never send an email");
    expect(core).toContain("do_not_contact");
    expect(core).toContain("## Record");
    expect(core).toContain("sales_report");
  });

  it("writes and checks outreach against sales_offer and the compliance rules", () => {
    for (const name of ["sales-writing", "sales-review", "sales-deal-analysis"]) {
      expect(bodyOf(name), name).toContain("sales_offer");
    }
    expect(bodyOf("sales-writing")).toContain("[QC]");
    expect(bodyOf("sales-writing")).toContain("STOP");
    expect(bodyOf("sales-review")).toContain("pass");
    expect(bodyOf("sales-review")).toContain("concerns");
    expect(bodyOf("sales-review")).toContain("fail");
  });

  it("names only commands the squad can run", () => {
    expect(bodyOf("sales-scouting")).toContain("multica sales collect");
    expect(bodyOf("sales-source-builder")).toContain("multica sales probe");
    expect(bodyOf("sales-delivery-core")).toContain("multica knowledge search");
    expect(bodyOf("sales-squad-lead")).toContain("multica proposal create");
  });

  it("keeps the source states the spec names, and every one has a stated next step", () => {
    const builder = bodyOf("sales-source-builder");
    for (const state of ["active", "needs_connector", "manual_only", "blocked", "degraded", "disabled"]) {
      expect(builder, state).toContain(`\`${state}\``);
    }
  });

  it("never lets a skill raise a cap or contact anyone on its own", () => {
    for (const name of SALES_SKILL_NAMES) {
      expect(bodyOf(name), name).not.toMatch(/\bpost (it|the reply) yourself\b/i);
    }
    expect(bodyOf("sales-scouting")).toContain("do not raise a cap");
  });

  it("hands a won deal off with the existing request command, and never a deal that is not won", () => {
    const lead = bodyOf("sales-squad-lead");
    expect(lead).toContain("multica squad handoff <request-id>");
    expect(lead).toContain("Never hand off a deal that is not `won`");
  });

  it("drafts to Gmail only when it is connected, and never sends", () => {
    const writing = bodyOf("sales-writing");
    expect(writing).toContain("Gmail drafts");
    expect(writing).toContain("Never send the draft");
    expect(writing).toContain("do nothing");
  });

  it("ships the reference files the procedures point to", () => {
    expect(skill("sales-delivery-core").files.map((f) => f.path)).toContain("reference.md");
    expect(skill("sales-scouting").files.map((f) => f.path)).toContain("reference.md");
    expect(skill("sales-writing").files.map((f) => f.path)).toContain("reference.md");
  });
});
