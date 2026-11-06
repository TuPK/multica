// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import {
  SDLC_AUTO_WORKFLOW,
  SDLC_SKILL_NAMES,
  SQUAD_ROLES,
  SQUAD_TEMPLATES,
  type SquadTemplate,
} from "./templates";

const template = (id: SquadTemplate["id"]) =>
  SQUAD_TEMPLATES.find((t) => t.id === id)!;

describe("SQUAD_TEMPLATES", () => {
  it("gives every template a unique id and a squad name", () => {
    const ids = SQUAD_TEMPLATES.map((t) => t.id);

    expect(new Set(ids).size).toBe(ids.length);
    for (const t of SQUAD_TEMPLATES) expect(t.name.trim()).not.toBe("");
  });

  it("starts every squad from a leader and only known roles", () => {
    for (const t of SQUAD_TEMPLATES) {
      expect(t.roles, t.id).toContain(t.leader);
      expect(t.roles[0], t.id).toBe(t.leader);
      expect(new Set(t.roles).size, t.id).toBe(t.roles.length);
      for (const role of t.roles) expect(SQUAD_ROLES[role], `${t.id}: ${role}`).toBeDefined();
    }
  });

  it("builds every role from an existing agent instruction template", () => {
    const ids = new Set(AGENT_INSTRUCTION_TEMPLATES.map((t) => t.id));
    const names = new Set<string>();

    for (const role of Object.values(SQUAD_ROLES)) {
      expect(ids.has(role.agentTemplate), role.key).toBe(true);
      expect([...role.description].length, role.key).toBeLessThanOrEqual(255);
      expect(names.has(role.name), role.name).toBe(false);
      names.add(role.name);
    }
  });

  it("hands every workflow step to a role its squad has", () => {
    for (const t of SQUAD_TEMPLATES) {
      if (!t.workflow) continue;
      for (const step of t.workflow.steps) {
        if (step.handler) expect(t.roles, t.id).toContain(step.handler);
      }
    }
  });

  it("keeps each workflow step within the step instruction limit", () => {
    for (const step of SDLC_AUTO_WORKFLOW.steps) {
      expect([...step.instructions].length).toBeLessThanOrEqual(8000);
    }
  });

  it("hands each planning status to the member who owns that stage", () => {
    const owners = Object.fromEntries(
      SDLC_AUTO_WORKFLOW.steps
        .filter((s) => "name" in s.status)
        .map((s) => [("name" in s.status ? s.status.name : ""), s.handler]),
    );

    expect(owners).toEqual({
      Analysis: "analyst",
      Design: "ux",
      Architecture: "architect",
      "Plan review": "reviewer",
      "Security review": "security",
      "Release prep": "devops",
      "Stage done": null,
      "Plan ready": null,
      QA: "qa",
    });
  });

  it("ends every planning step at Stage done, which counts as finished", () => {
    const planning = SDLC_AUTO_WORKFLOW.steps.filter(
      (s) => s.handler !== null && "name" in s.status && s.status.name !== "QA",
    );

    expect(planning.length).toBe(6);
    for (const step of planning) expect(step.next).toBe("stage_done");
    const done = SDLC_AUTO_WORKFLOW.steps.find((s) => "name" in s.status && s.status.name === "Stage done");
    expect(done?.status).toEqual({ name: "Stage done", category: "done" });
  });

  it("keeps the build flow on the developer, reviewer, and QA, and each rejected step goes back", () => {
    const byKey = (key: string) => SDLC_AUTO_WORKFLOW.steps.find((s) => "key" in s.status && s.status.key === key)!;

    expect(byKey("todo").handler).toBe("developer");
    expect(byKey("in_progress")).toMatchObject({ handler: "developer", back: "todo", next: "in_review" });
    expect(byKey("in_review")).toMatchObject({ handler: "reviewer", back: "in_progress", next: "qa" });
  });

  it("names only status keys the workflow defines or the built-in statuses", () => {
    const defined = new Set([
      ...SDLC_AUTO_WORKFLOW.steps.map((s) => ("key" in s.status ? s.status.key : s.status.name.toLowerCase().replace(/ /g, "_"))),
      "done",
      "cancelled",
    ]);
    for (const step of SDLC_AUTO_WORKFLOW.steps) {
      for (const ref of [step.next, step.back]) {
        if (ref) expect(defined.has(ref), ref).toBe(true);
      }
    }
  });

  it("hands a blocked build issue to the lead, which sends it back to In Progress", () => {
    const blocked = SDLC_AUTO_WORKFLOW.steps.find((s) => "key" in s.status && s.status.key === "blocked");

    expect(blocked?.handler).toBe("lead");
    expect(blocked?.back).toBe("in_progress");
    expect(blocked?.instructions).toContain("planning sub-issue");
    expect(blocked?.instructions).toContain("Do not leave it here");
  });
});

describe("market research template", () => {
  const market = template("market_research");

  it("leads with the research lead and seats the squad on a Research floor", () => {
    expect(market.leader).toBe("research_lead");
    expect(market.roles).toEqual(["research_lead", "scout", "competitor", "critic"]);
    expect(market.floorName).toBe("Research");
    expect(market.workflow).toBeNull();
  });

  it("runs a Monday-morning routine that opens a dated issue for the squad", () => {
    const routine = market.routine!;

    expect(routine.cron).toBe("0 8 * * 1");
    expect(routine.issueTitleTemplate).toContain("{{date}}");
    expect(routine.description).toContain("File zero to three proposals");
  });

  it("gives the critic the review role and keeps proposals behind it", () => {
    expect(market.instructions).toContain("multica proposal create --squad");
    expect(market.instructions).toContain("after the critic has reviewed them");
    expect(market.instructions).toContain("Do not write product code");
  });

  it("keeps the SDLC templates without a routine or a floor name", () => {
    for (const t of SQUAD_TEMPLATES.filter((x) => x.id !== "market_research")) {
      expect(t.routine, t.id).toBeNull();
      expect(t.floorName, t.id).toBeNull();
      expect(t.leader, t.id).toBe("lead");
    }
  });
});

describe("squad instructions", () => {
  it("point every SDLC squad at the built-in procedure skills, not a copy of it", () => {
    for (const id of ["sdlc_greenfield", "sdlc_maintenance"] as const) {
      const instructions = template(id).instructions;
      expect(instructions, id).toContain("sdlc-delivery-core");
      expect(instructions, id).toContain("sdlc-squad-lead");
      expect(instructions, id).toContain("Delegate only to the members listed on your Squad Roster");
    }
  });

  it("import the built-in SDLC skills into every SDLC squad and no other", () => {
    expect(template("sdlc_greenfield").builtinSkills).toEqual(SDLC_SKILL_NAMES);
    expect(template("sdlc_maintenance").builtinSkills).toEqual(SDLC_SKILL_NAMES);
    expect(template("market_research").builtinSkills).toEqual([]);
  });

  it("names the Greenfield build project and its route, and the Maintenance triage", () => {
    expect(template("sdlc_greenfield").instructions).toContain('"SDLC Auto" workflow');
    expect(template("sdlc_greenfield").instructions).toContain("route (A, B, C, or D)");
    expect(template("sdlc_maintenance").instructions).toContain("route and a risk tier");
  });

  it("keeps the market research squad's own delegation rules", () => {
    const research = template("market_research").instructions;
    expect(research).toContain("Do not @mention a member");
    expect(research).toContain("--assignee-id <member id>");
  });

  it("only name BMAD skills that exist, in the instructions and in the skills", () => {
    const shipped = new Set([
      "bmad-architecture",
      "bmad-build",
      "bmad-build-auto",
      "bmad-code-review",
      "bmad-correct-course",
      "bmad-create-epics-and-stories",
      "bmad-prd",
      "bmad-prfaq",
      "bmad-product-brief",
      "bmad-project-context",
      "bmad-qa-generate-e2e-tests",
      "bmad-retrospective",
      "bmad-sprint-planning",
      "bmad-spec",
      "bmad-ux",
    ]);
    const texts = [
      ...SQUAD_TEMPLATES.map((t) => [t.id, t.instructions] as const),
      ...SDLC_BUILTIN_SKILLS.map((skill) => [skill.name, skill.content] as const),
    ];

    for (const [id, text] of texts) {
      for (const skill of text.match(/\bbmad-[a-z0-9]+(?:-[a-z0-9]+)*/g) ?? []) {
        expect(shipped.has(skill), `${id}: ${skill}`).toBe(true);
      }
    }
  });
});
