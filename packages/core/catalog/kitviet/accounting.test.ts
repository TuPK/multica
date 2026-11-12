// @vitest-environment node

// Plan S4 for the Accounting & Tax template: roles, one lead, desk size, floor, skills that exist
// in the pack, charter shape, and the routine's cron and paused start.

import { describe, expect, it } from "vitest";
import { composeKvAgentInstructions, KV_ROLE_KINDS } from "../../squads/kitviet-agent-instructions";
import { COMPANY_FLOOR_NAMES } from "../../squads/company-template";
import { findKitvietRole, kitvietSquad } from "../../squads/kitviet-squad-roles";
import { loadKitvietRoleInstructions } from "../../squads/kitviet-packs";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../generated/kitviet/accounting";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../generated/kitviet/common";
import type { KitvietSquadRole } from "../generated/kitviet/types";
import type { SquadRole } from "../../squads/templates";
import { SQUAD_ROLES, SQUAD_TEMPLATES } from "../../squads/templates";

// The squad is catalog data (catalog/squads/kv_accounting), assembled into SQUAD_TEMPLATES and SQUAD_ROLES.
const KV_ACCOUNTING_ROLES = Object.fromEntries(SQUAD_TEMPLATES.find((t) => t.id === "kv_accounting")!.roles.map((key) => [key, SQUAD_ROLES[key]])) as typeof SQUAD_ROLES;
const KV_ACCOUNTING_TEMPLATE = SQUAD_TEMPLATES.find((t) => t.id === "kv_accounting")!;

const template = KV_ACCOUNTING_TEMPLATE;
const squad = kitvietSquad("kv_accounting");

// Looks up a table by a role key taken from the squad data, failing with the key when it is absent.
function lookup<T>(table: Record<string, T | undefined>, key: string): T {
  const value = table[key];
  if (value === undefined) throw new Error(`no entry for ${key}`);
  return value;
}
const roleEntry = (key: string): SquadRole => lookup<SquadRole>(KV_ACCOUNTING_ROLES, key);
// The generated role of a key: its kind and header, from the names-only squad data.
const generatedOf = (key: string): KitvietSquadRole => {
  const found = findKitvietRole(key);
  if (!found) throw new Error(`no generated role ${key}`);
  return found.role;
};
// The instructions an agent of this role is created with (loaded with its pack).
async function instructionOf(key: string): Promise<string> {
  const text = await loadKitvietRoleInstructions(key);
  if (text === undefined) throw new Error(`no Kit Viet instructions for ${key}`);
  return text;
}

// A squad loads skills from every pack (quickstart: loadKitvietPacks), so the shared pack counts.
const packSkillNames = new Set([...COMMON_SKILLS, ...KITVIET_BUILTIN_SKILLS].map((skill) => skill.name));
const roleSkillLists = Object.values(template.roleSkills ?? {});
const members = template.roles.filter((role) => role !== template.leader);

describe("Accounting & Tax template", () => {
  it("is the kv_accounting template with the generated squad name", () => {
    expect(template.id).toBe("kv_accounting");
    expect(template.name).toBe(squad.name);
    expect(template.description.trim()).not.toBe("");
    expect(template.workflow).toBeNull();
  });

  it("lists every role of the generated squad, once, with a role entry each", () => {
    expect(template.roles).toEqual(squad.roles.map((role) => role.key));
    expect(new Set(template.roles).size).toBe(template.roles.length);
    for (const key of template.roles) {
      const role = roleEntry(key);
      expect(role.key).toBe(key);
      expect(role.name).toBe(generatedOf(key).name);
      expect(role.description.trim()).not.toBe("");
      expect(role.role.trim()).not.toBe("");
    }
  });

  it("gives each role a unique name and the instruction kind the pack generated", () => {
    const names = template.roles.map((key) => roleEntry(key).name);
    expect(new Set(names).size).toBe(names.length);
    for (const key of template.roles) {
      expect(KV_ROLE_KINDS).toContain(roleEntry(key).agentTemplate);
      expect(roleEntry(key).agentTemplate).toBe(generatedOf(key).kind);
    }
  });

  it("has exactly one lead, and the leader is that lead", () => {
    const leads = template.roles.filter((key) => roleEntry(key).agentTemplate === "kv_lead");
    expect(leads).toEqual(["acct_lead"]);
    expect(template.leader).toBe("acct_lead");
    expect(template.roles[0]).toBe(template.leader);
  });

  it("stays within the desk limit: one lead and at most eight members", () => {
    expect(template.roles.length).toBeLessThanOrEqual(9);
    expect(members.length).toBeLessThanOrEqual(8);
  });

  it("names a floor that the company template defines", () => {
    expect(Object.values(COMPANY_FLOOR_NAMES)).toContain(template.floorName);
    expect(template.floorName).toBe("Governance");
  });

  it("uses only skills that exist in the pack", () => {
    for (const name of template.builtinSkills) expect(packSkillNames.has(name), name).toBe(true);
    for (const name of template.leaderSkills ?? []) expect(packSkillNames.has(name), name).toBe(true);
    for (const name of roleSkillLists.flat()) expect(packSkillNames.has(name), name).toBe(true);
    expect(new Set(template.builtinSkills).size).toBe(template.builtinSkills.length);
  });

  it("takes its skill sets from the generated squad data", () => {
    expect(template.builtinSkills).toEqual(squad.builtinSkills);
    expect(template.leaderSkills).toEqual(squad.leaderSkills);
    expect(template.roleSkills).toEqual(squad.roleSkills);
  });

  it("attaches at most six role skills per role, and only to roles of this squad", () => {
    for (const [role, skills] of Object.entries(template.roleSkills ?? {})) {
      expect(template.roles, role).toContain(role);
      expect(skills.length, role).toBeLessThanOrEqual(6);
    }
  });

  it("writes a charter with a mission, responsibilities, outputs, boundaries, escalation and intake", () => {
    const { charter } = template;
    expect(charter.mission.trim()).not.toBe("");
    expect(charter.responsibilities.length).toBeGreaterThan(0);
    const keys = charter.responsibilities.map((item) => item.key);
    expect(new Set(keys).size).toBe(keys.length);
    for (const item of charter.responsibilities) expect(item.text.trim()).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) {
      expect(["issue", "proposal", "kb_document", "report"]).toContain(output.kind);
    }
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake.trim()).not.toBe("");
  });

  it("says in the charter that the runtime needs Bash and Python, and carries the 8% VAT register note", () => {
    const charterText = JSON.stringify(template.charter);
    expect(charterText).toContain("Bash");
    expect(charterText).toContain("Python");
    expect(charterText).toContain("8%");
    expect(charterText).toContain("31/12/2026");
  });

  it("has one routine, paused, weekly on Monday at eight", () => {
    expect(template.routines).toHaveLength(1);
    const routine = template.routines[0];
    if (!routine) throw new Error("the accounting template has no routine");
    expect(routine.title).toBe("Lịch hạn nộp thuế");
    expect(routine.cron).toBe("0 8 * * 1");
    expect(routine.cron.trim().split(/\s+/)).toHaveLength(5);
    expect(routine.startPaused).toBe(true);
    expect(routine.issueTitleTemplate).toContain("{{date}}");
    expect(routine.description.trim()).not.toBe("");
  });

  it("starts the lead from its generated header and the squad rules", async () => {
    expect(template.instructions).toBe(await instructionOf("acct_lead"));
    expect(template.instructions.startsWith(generatedOf("acct_lead").header)).toBe(true);
    expect(template.instructions).toContain("Bash and Python");
  });

  it("composes each member's text from its header, its kind text and its generated body", async () => {
    for (const key of members) {
      const generated = generatedOf(key);
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;
      expect(body, key).toBeTruthy();
      expect(await instructionOf(key), key).toBe(
        composeKvAgentInstructions({ kind: generated.kind, header: generated.header, body }),
      );
    }
  });
});
