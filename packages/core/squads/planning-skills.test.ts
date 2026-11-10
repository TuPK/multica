// @vitest-environment node
import { describe, expect, it } from "vitest";
import { PLANNING_ROLE_AGENT_INSTRUCTIONS } from "./planning-agent-instructions";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import { PLANNING_SKILL_NAMES, SQUAD_TEMPLATES } from "./templates";

const get = (name: string) => SDLC_BUILTIN_SKILLS.find((s) => s.name === name)!;

const ROLE_SKILLS = [
  "planning-squad-lead",
  "planning-research",
  "planning-product",
  "planning-ux",
  "planning-architecture",
  "planning-readiness",
];

describe("built-in planning skills", () => {
  it("makes every role skill load the shared core first, say when to use it, and end with a self-check", () => {
    for (const name of ROLE_SKILLS) {
      const skill = get(name);
      expect(skill.description, name).toMatch(/^Use when /);
      expect(skill.content, name).toContain("planning-delivery-core");
      expect(skill.content, name).toContain("Self-check before you report");
    }
  });

  it("keeps the unattended rules and the in-place editing rules in the shared core", () => {
    const core = get("planning-delivery-core").content;
    expect(core).toContain("Do not wait for an approval");
    expect(core).toContain("Keep ids stable");
    expect(core).toContain("Delivered work is fixed");
    expect(core).toContain("Change log");
    expect(core).toContain("Blocker-class: source | requirement | design | ux | tooling | scope-reversal | irreversible | capability");
  });

  it("gives the lead the queue commands, the impact map, and the handoff packet, and keeps it from writing documents", () => {
    const lead = get("planning-squad-lead");
    expect(lead.files.map((f) => f.path)).toEqual(["reference.md"]);
    expect(lead.content).toContain("You never write the documents");
    expect(lead.content).toContain("multica issue queue <issue> --after <issue>");
    expect(lead.content).toContain("Folder mode `in_place`: one issue at a time, always");
    for (const status of ["research", "brief", "prd", "design", "architecture", "epics", "plan_review"]) {
      expect(lead.content, status).toContain("`" + status + "`");
    }
    const reference = lead.files[0]!.content;
    expect(reference).toContain("# Impact map: <request identifier> <title>");
    expect(reference).toContain("## Planning handoff");
    expect(reference).toContain("Delivered and untouched:");
  });

  it("ends the lead's run with one handoff request that follows the platform's answer", () => {
    const lead = get("planning-squad-lead").content;
    expect(lead).toContain("multica squad handoff <request>");
    for (const answer of ["handed_off", "pending", "off", "declined"]) expect(lead, answer).toContain("`" + answer + "`");
    expect(lead).toContain("Never create the build squad's request yourself");
    expect(PLANNING_ROLE_AGENT_INSTRUCTIONS.plan_lead).toContain("multica squad handoff <request>");
  });

  it("teaches the SDLC lead to read a planning handoff as a Plan A request", () => {
    const sdlc = get("sdlc-squad-lead").content;
    expect(sdlc).toContain("### A request from a planning squad");
    expect(sdlc).toContain("[Planning handoff]");
    expect(sdlc).toContain("A packet is a claim");
  });

  it("keeps the rules that stop a role from doing another role's work", () => {
    expect(get("planning-readiness").content).toContain("You do not edit");
    expect(get("planning-readiness").content).toContain("You changed no file");
    expect(get("planning-research").content).toContain("You do not");
    expect(get("planning-architecture").content).toContain("Rewrite an accepted ADR");
    expect(get("planning-ux").content).toContain("Add a screen or a flow that no requirement needs");
    expect(get("planning-product").content).toContain("Remove or renumber an epic");
  });

  it("names for each role only skills the planning squad carries", () => {
    const carried = new Set(PLANNING_SKILL_NAMES.map((n) => n.split(":").pop()!));
    for (const name of ROLE_SKILLS) {
      const text = [get(name).content, ...get(name).files.map((f) => f.content)].join("\n");
      for (const skill of text.match(/\b(?:bmad-[a-z0-9]+(?:-[a-z0-9]+)*|planning-[a-z]+(?:-[a-z]+)*|frontend-design)\b/g) ?? []) {
        expect(carried.has(skill), `${name}: ${skill}`).toBe(true);
      }
    }
  });
});

describe("planning agent instructions", () => {
  it("has one text per planning role, each loading the core and its role skill", () => {
    const planning = SQUAD_TEMPLATES.find((t) => t.id === "bmad_planning")!;
    expect(Object.keys(PLANNING_ROLE_AGENT_INSTRUCTIONS).sort()).toEqual([...planning.roles].sort());
    const names = new Set(SDLC_BUILTIN_SKILLS.map((s) => s.name));
    for (const [role, text] of Object.entries(PLANNING_ROLE_AGENT_INSTRUCTIONS)) {
      expect(text, role).toContain("planning-delivery-core");
      for (const skill of text.match(/planning-[a-z]+(?:-[a-z]+)*/g) ?? []) {
        expect(names.has(skill), `${role}: ${skill}`).toBe(true);
      }
    }
  });

  it("keeps the lead from writing documents or moving the request", () => {
    const lead = PLANNING_ROLE_AGENT_INSTRUCTIONS.plan_lead;
    expect(lead).toContain("Never change the status or the assignee of the request");
    expect(lead).toContain("Never write research, a brief, a PRD");
  });
});
