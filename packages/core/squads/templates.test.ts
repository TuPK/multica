// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import {
  PLANNING_AUTO_WORKFLOW,
  PLANNING_SKILL_NAMES,
  SDLC_AUTO_WORKFLOW,
  SALES_SKILL_NAMES,
  SDLC_SKILL_NAMES,
  SQUAD_ROLES,
  SQUAD_TEMPLATES,
  type SquadTemplate,
  type SquadWorkflowStep,
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
      Environment: "devops",
      "Stage done": null,
      "Plan ready": null,
      QA: "qa",
    });
  });

  it("ends every planning step at Stage done, which counts as finished", () => {
    const planning = SDLC_AUTO_WORKFLOW.steps.filter(
      (s) => s.handler !== null && "name" in s.status && s.status.name !== "QA",
    );

    expect(planning.length).toBe(7);
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
    expect(blocked?.instructions).toContain("sdlc-squad-lead");
    expect(blocked?.instructions).toContain("unblock issue");
    expect(blocked?.instructions).toContain("do not leave the issue here");
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
    const routine = market.routines[0]!;

    expect(market.routines).toHaveLength(1);
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
    for (const t of SQUAD_TEMPLATES.filter((x) => x.id.startsWith("sdlc_"))) {
      expect(t.routines, t.id).toEqual([]);
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
    // The BMAD skills the bundle ships, by their short names (`bmad-method:bmad-prd` is `bmad-prd`).
    const shipped = new Set(
      SDLC_BUILTIN_SKILLS.map((skill) => skill.name.split(":").pop()!).filter((name) => name.startsWith("bmad-")),
    );
    const texts = [
      ...SQUAD_TEMPLATES.map((t) => [t.id, t.instructions] as const),
      // The SDLC procedures this repo owns. The attached BMAD skills carry their own text,
      // which names BMAD skills the squad may not have, so they are not checked here.
      ...SDLC_BUILTIN_SKILLS.filter((skill) => skill.name.startsWith("sdlc-") || skill.name.startsWith("planning-")).map((skill) => [skill.name, skill.content] as const),
    ];

    for (const [id, text] of texts) {
      for (const skill of text.match(/\bbmad-[a-z0-9]+(?:-[a-z0-9]+)*/g) ?? []) {
        expect(shipped.has(skill), `${id}: ${skill}`).toBe(true);
      }
    }
  });
});

describe("bmad planning template", () => {
  const planning = template("bmad_planning");
  const statusKey = (s: SquadWorkflowStep) =>
    "key" in s.status ? s.status.key : s.status.name.toLowerCase().replace(/ /g, "_");

  it("leads with the planning lead and has seven members with names no other squad uses", () => {
    expect(planning.leader).toBe("plan_lead");
    expect(planning.roles).toEqual([
      "plan_lead",
      "plan_researcher",
      "plan_analyst",
      "plan_pm",
      "plan_ux",
      "plan_architect",
      "plan_reviewer",
    ]);
    expect(planning.routines).toEqual([]);
    expect(planning.floorName).toBeNull();
    const others = new Set(
      Object.values(SQUAD_ROLES)
        .filter((r) => !planning.roles.includes(r.key))
        .map((r) => r.name),
    );
    for (const role of planning.roles) {
      expect(SQUAD_ROLES[role].name, role).not.toMatch(/^BMAD/);
      expect(others.has(SQUAD_ROLES[role].name), role).toBe(false);
    }
  });

  it("hands each planning status to the member who owns that document", () => {
    const owners = Object.fromEntries(
      PLANNING_AUTO_WORKFLOW.steps
        .filter((s) => "name" in s.status)
        .map((s) => [("name" in s.status ? s.status.name : ""), s.handler]),
    );
    expect(owners).toEqual({
      Research: "plan_researcher",
      Brief: "plan_analyst",
      PRD: "plan_pm",
      Design: "plan_ux",
      Architecture: "plan_architect",
      Epics: "plan_pm",
      "Plan review": "plan_reviewer",
      "Stage done": null,
      "Plan ready": null,
    });
    const blocked = PLANNING_AUTO_WORKFLOW.steps.find((s) => "key" in s.status && s.status.key === "blocked");
    expect(blocked?.handler).toBe("plan_lead");
    expect(blocked?.instructions).toContain("planning-squad-lead");
  });

  it("ends every stage at Stage done and names only statuses the workflow defines", () => {
    const defined = new Set([...PLANNING_AUTO_WORKFLOW.steps.map(statusKey), "done", "cancelled"]);
    const stages = PLANNING_AUTO_WORKFLOW.steps.filter((s) => s.handler !== null && "name" in s.status);
    expect(stages.length).toBe(7);
    for (const step of stages) expect(step.next).toBe("stage_done");
    for (const step of PLANNING_AUTO_WORKFLOW.steps) {
      for (const ref of [step.next, step.back]) if (ref) expect(defined.has(ref), ref).toBe(true);
      expect([...step.instructions].length).toBeLessThanOrEqual(8000);
      if (step.handler) expect(planning.roles).toContain(step.handler);
    }
  });

  it("imports its own skills, and the SDLC squads import none of them", () => {
    expect(planning.builtinSkills).toEqual(PLANNING_SKILL_NAMES);
    const shipped = new Set(SDLC_BUILTIN_SKILLS.map((s) => s.name));
    for (const name of PLANNING_SKILL_NAMES) expect(shipped.has(name), name).toBe(true);
    const planningOnly = [
      ...PLANNING_SKILL_NAMES.filter((n) => n.startsWith("planning-") || n.startsWith("bmad-toolbox:")),
      "frontend-design:frontend-design",
    ];
    for (const name of planningOnly) expect(SDLC_SKILL_NAMES, name).not.toContain(name);
    for (const name of SDLC_SKILL_NAMES) expect(shipped.has(name), name).toBe(true);
  });

  it("points the leader at the planning procedure skills and keeps the squad to planning documents", () => {
    expect(planning.instructions).toContain("planning-delivery-core");
    expect(planning.instructions).toContain("planning-squad-lead");
    expect(planning.instructions).toContain("Delegate only to the members listed on your Squad Roster");
    expect(planning.instructions).toContain("writes no product code");
    expect(planning.workflow).toBe(PLANNING_AUTO_WORKFLOW);
  });
});

describe("sales loop template", () => {
  const sales = template("sales_loop");

  it("leads with the sales lead and names five agents with the Sales prefix", () => {
    expect(sales.leader).toBe("sales_lead");
    expect(sales.roles).toEqual(["sales_lead", "sales_scout", "sales_writer", "sales_analyst", "sales_checker"]);
    for (const role of sales.roles) {
      expect(SQUAD_ROLES[role].name.startsWith("Sales "), role).toBe(true);
    }
    expect(sales.floorName).toBeNull();
    expect(sales.workflow).toBeNull();
  });

  it("creates three routines with the weekly and daily schedules of the spec", () => {
    expect(sales.routines.map((r) => r.title)).toEqual(["Weekly scan", "Weekly review", "Source health"]);
    expect(sales.routines.map((r) => r.cron)).toEqual(["0 8 * * 1", "0 20 * * 0", "0 7 * * *"]);
    for (const r of sales.routines) expect(r.issueTitleTemplate, r.title).toMatch(/\{\{date\}\}|^Source health/);
  });

  it("keeps its agent names apart from every other role's name", () => {
    const others = new Set(
      Object.values(SQUAD_ROLES)
        .filter((r) => !sales.roles.includes(r.key))
        .map((r) => r.name),
    );
    for (const role of sales.roles) expect(others.has(SQUAD_ROLES[role].name), role).toBe(false);
  });

  it("imports the sales skills and states the no-send rule in its instructions", () => {
    expect(sales.builtinSkills).toEqual(SALES_SKILL_NAMES);
    expect(sales.instructions).toContain("sales-delivery-core");
    expect(sales.instructions).toContain("sales-squad-lead");
    expect(sales.instructions).toContain("Nothing is sent by the squad");
    expect(sales.charter.boundaries).toContain("Never sends");
  });
});
