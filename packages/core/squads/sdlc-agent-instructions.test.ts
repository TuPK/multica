// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { SDLC_ROLE_AGENT_INSTRUCTIONS, type SdlcRoleKey } from "./sdlc-agent-instructions";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";

const roles = Object.keys(SDLC_ROLE_AGENT_INSTRUCTIONS) as SdlcRoleKey[];

describe("SDLC agent instructions", () => {
  it("covers every SDLC role the squad creates", () => {
    expect(roles.sort()).toEqual(
      ["architect", "analyst", "devops", "developer", "fixer", "lead", "qa", "reviewer", "security", "ux"].sort(),
    );
  });

  it("points each agent at a built-in skill that exists, and at the shared core", () => {
    const names = new Set(SDLC_BUILTIN_SKILLS.map((s) => s.name));
    for (const role of roles) {
      const text = SDLC_ROLE_AGENT_INSTRUCTIONS[role];
      expect(text, role).toContain("sdlc-delivery-core");
      const named = [...new Set(text.match(/sdlc-[a-z-]+/g) ?? [])].filter((n) => n !== "sdlc-delivery-core");
      expect(named.length, role).toBeGreaterThan(0);
      for (const skill of named) expect(names.has(skill), `${role}: ${skill}`).toBe(true);
    }
  });

  it("stays short: the procedure lives in the skills, not in the agent", () => {
    for (const role of roles) {
      expect(SDLC_ROLE_AGENT_INSTRUCTIONS[role].length, role).toBeLessThan(2000);
    }
  });

  it("does not change the shared templates other squads use", () => {
    const shared = AGENT_INSTRUCTION_TEMPLATES.find((t) => t.id === "architect")!;
    expect(shared.instructions).toContain("You are the Architect.");
  });
});
