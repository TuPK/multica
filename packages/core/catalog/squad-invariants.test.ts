// @vitest-environment node
import { describe, expect, it } from "vitest";
import { SUPPORTED_LOCALES } from "../i18n/types";
import { SQUAD_ROLES, SQUAD_TEMPLATES, SQUAD_TEMPLATE_CATEGORIES } from "../squads/templates";
import { SDLC_BUILTIN_SKILLS } from "./generated/skills-builtin";
import { catalogText } from "./i18n";

// Every squad of the catalog, checked the same way, so a squad added under catalog/squads is covered
// without a test of its own. The per-squad Kit Viet tests next to this file keep the facts that are
// specific to one squad.
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];
const CRON_FIELD = /^[\d*/,\-?LW#]+$/;
const BUNDLED = new Set(SDLC_BUILTIN_SKILLS.map((skill) => skill.name));

describe("catalog squads", () => {
  it("lists each squad once, in a known collection", () => {
    const ids = SQUAD_TEMPLATES.map((t) => t.id);
    expect(new Set(ids).size).toBe(ids.length);
    for (const t of SQUAD_TEMPLATES) expect(SQUAD_TEMPLATE_CATEGORIES, t.id).toContain(t.category);
  });

  it("gives every collection and every squad a title in every supported locale, with no fallback", () => {
    for (const locale of SUPPORTED_LOCALES) {
      for (const category of SQUAD_TEMPLATE_CATEGORIES) expect(catalogText("collection", category, locale).title.trim(), `${category} ${locale}`).not.toBe("");
      for (const t of SQUAD_TEMPLATES) {
        const text = catalogText("squad", t.id, locale);
        expect(text.title.trim(), `${t.id} ${locale}`).not.toBe("");
        expect(text.description?.trim(), `${t.id} ${locale}`).toBeTruthy();
      }
    }
  });

  it("has a leader that is the first role, and roles listed once", () => {
    for (const t of SQUAD_TEMPLATES) {
      expect(new Set(t.roles).size, t.id).toBe(t.roles.length);
      expect(t.roles, t.id).toContain(t.leader);
      expect(t.roles[0], t.id).toBe(t.leader);
      for (const key of t.roles) expect(SQUAD_ROLES[key], `${t.id} ${key}`).toBeDefined();
    }
  });

  it("names every role and gives it a description and a roster line", () => {
    for (const t of SQUAD_TEMPLATES) {
      for (const key of t.roles) {
        const role = SQUAD_ROLES[key];
        expect(role.key).toBe(key);
        expect(role.name.trim(), `${t.id} ${key}`).not.toBe("");
        expect(role.description.trim(), `${t.id} ${key}`).not.toBe("");
        expect(role.role.trim(), `${t.id} ${key}`).not.toBe("");
      }
    }
  });

  it("gives each agent name to one role only, so a rerun finds the right agent", () => {
    const owner = new Map<string, string>();
    for (const [key, role] of Object.entries(SQUAD_ROLES)) {
      expect(owner.get(role.name), `${role.name}: ${owner.get(role.name)} and ${key}`).toBeUndefined();
      owner.set(role.name, key);
    }
  });

  it("has a one-line description and instructions", () => {
    for (const t of SQUAD_TEMPLATES) {
      expect(t.description.trim(), t.id).not.toBe("");
      expect(t.description, t.id).not.toContain("\n");
      expect(t.instructions.trim(), t.id).not.toBe("");
    }
  });

  it("has a valid charter and five-field routines", () => {
    for (const t of SQUAD_TEMPLATES) {
      const c = t.charter;
      expect(c.mission.trim(), t.id).not.toBe("");
      expect(c.responsibilities.length, t.id).toBeGreaterThan(0);
      expect(new Set(c.responsibilities.map((r) => r.key)).size, t.id).toBe(c.responsibilities.length);
      for (const output of c.outputs) expect(OUTPUT_KINDS, `${t.id} ${output.key}`).toContain(output.kind);
      expect(c.escalation.trim(), t.id).not.toBe("");
      expect(c.intake.trim(), t.id).not.toBe("");
      for (const routine of t.routines) expect(routine.cron.trim().split(/\s+/).length, `${t.id} ${routine.title}`).toBe(5);
      for (const routine of t.routines) for (const field of routine.cron.trim().split(/\s+/)) expect(field, `${t.id} ${routine.title}`).toMatch(CRON_FIELD);
    }
  });

  it("names only skills the bundle carries, and extra skills only for roles of the squad", () => {
    for (const t of SQUAD_TEMPLATES) {
      expect(new Set(t.builtinSkills).size, t.id).toBe(t.builtinSkills.length);
      const attached = [...t.builtinSkills, ...(t.leaderSkills ?? []), ...Object.values(t.roleSkills ?? {}).flat()] as string[];
      // A Kit Viet squad's skills (kv--*) ship in its own pack, not in the SDLC bundle.
      for (const name of attached.filter((n) => !n.startsWith("kv--"))) expect(BUNDLED.has(name), `${t.id} ${name}`).toBe(true);
      for (const key of Object.keys(t.roleSkills ?? {})) expect(t.roles as string[], `${t.id} ${key}`).toContain(key);
    }
  });

  it("hands every workflow step to a role of the squad", () => {
    for (const t of SQUAD_TEMPLATES) {
      if (!t.workflow) continue;
      for (const step of t.workflow.steps) if (step.handler) expect(t.roles, `${t.id} ${String(step.handler)}`).toContain(step.handler);
    }
  });
});
