// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { COMPANY_FLOOR_NAMES } from "../../squads/company-template";
import { kitvietSquad } from "../../squads/kitviet-squad-roles";
import { loadKitvietRoleInstructions } from "../../squads/kitviet-packs";
import * as commonPack from "../generated/kitviet/common";
import * as legalPack from "../generated/kitviet/legal";
import { SQUAD_ROLES, SQUAD_TEMPLATES } from "../../squads/templates";

// The squad is catalog data (catalog/squads/kv_legal), assembled into SQUAD_TEMPLATES and SQUAD_ROLES.
const KV_LEGAL_TEMPLATE = SQUAD_TEMPLATES.find((t) => t.id === "kv_legal")!;
const LEGAL_ROLE_KEYS = SQUAD_TEMPLATES.find((t) => t.id === "kv_legal")!.roles;
const LEGAL_ROLES = Object.fromEntries(SQUAD_TEMPLATES.find((t) => t.id === "kv_legal")!.roles.map((key) => [key, SQUAD_ROLES[key]])) as typeof SQUAD_ROLES;

const t = KV_LEGAL_TEMPLATE;
const DESK_LIMIT = 9; // one lead and eight members (spec 5.1)
const CHARTER_OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];
const PACK_SKILL_NAMES = new Set(
  [...legalPack.KITVIET_BUILTIN_SKILLS, ...commonPack.KITVIET_BUILTIN_SKILLS].map((skill) => skill.name),
);

describe("KV_LEGAL_TEMPLATE", () => {
  it("names the squad, its floor and its template id", () => {
    expect(t.id).toBe("kv_legal");
    expect(t.name).toBe("Legal & Contracts (Pháp lý)");
    expect(t.description.trim()).not.toBe("");
    expect(t.floorName).toBe("Governance");
    expect(Object.values(COMPANY_FLOOR_NAMES)).toContain(t.floorName);
    expect(t.workflow).toBeNull();
  });

  it("lists every role, the same roles as the generated pack, with the leader first", () => {
    expect(t.roles).toEqual([...LEGAL_ROLE_KEYS]);
    expect([...t.roles].sort()).toEqual([...kitvietSquad("kv_legal").roles.map((role) => role.key)].sort());
    expect(t.roles[0]).toBe(t.leader);
    expect(new Set(t.roles).size).toBe(t.roles.length);
    for (const key of LEGAL_ROLE_KEYS) expect(LEGAL_ROLES[key].key, key).toBe(key);
  });

  it("has exactly one lead, and its roles fit the desk limit", () => {
    const leads = LEGAL_ROLE_KEYS.filter((key) => LEGAL_ROLES[key].agentTemplate === "kv_lead");
    expect(leads).toEqual([t.leader]);
    expect(t.roles.length).toBeLessThanOrEqual(DESK_LIMIT);
    expect(t.roles.length - 1).toBeLessThanOrEqual(8);
  });

  it("builds every role from an existing agent instruction template with a short description", () => {
    const ids = new Set(AGENT_INSTRUCTION_TEMPLATES.map((template) => template.id));
    const names = new Set<string>();
    for (const key of LEGAL_ROLE_KEYS) {
      const role = LEGAL_ROLES[key];
      expect(ids.has(role.agentTemplate), key).toBe(true);
      expect([...role.description].length, key).toBeLessThanOrEqual(255);
      expect(role.name.trim(), key).not.toBe("");
      expect(names.has(role.name), role.name).toBe(false);
      names.add(role.name);
    }
  });

  it("gives each agent its header, its kind text and, except the lead, its body", async () => {
    for (const key of LEGAL_ROLE_KEYS) {
      const text = await loadKitvietRoleInstructions(key);
      if (text === undefined) throw new Error(`no Kit Viet instructions for ${key}`);
      expect(text.startsWith("You are the "), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain("in a Legal & Contracts (Pháp lý) squad,");
      if (key !== t.leader) expect(text.length, key).toBeGreaterThan(500);
      if (LEGAL_ROLES[key].agentTemplate === "kv_gate") expect(text, key).toContain("VERDICT:");
    }
    expect(t.instructions).toBe(await loadKitvietRoleInstructions("legal_lead"));
    expect(t.instructions).toContain("Your job: turn a request into sub-issues");
  });

  it("names only built-in skills that exist in the pack", () => {
    for (const name of t.builtinSkills) expect(PACK_SKILL_NAMES.has(name), name).toBe(true);
    for (const name of t.leaderSkills ?? []) expect(PACK_SKILL_NAMES.has(name), name).toBe(true);
    for (const [key, skills] of Object.entries(t.roleSkills ?? {})) {
      expect(t.roles, key).toContain(key);
      for (const name of skills ?? []) expect(PACK_SKILL_NAMES.has(name), `${key}: ${name}`).toBe(true);
    }
    expect(new Set(t.builtinSkills).size).toBe(t.builtinSkills.length);
  });

  it("keeps the core skills and the leader skills in the generated pack", () => {
    expect(t.builtinSkills).toEqual(kitvietSquad("kv_legal").builtinSkills);
    expect(t.leaderSkills).toEqual(kitvietSquad("kv_legal").leaderSkills);
    expect(t.builtinSkills).toContain("kv--delivery-core");
    expect(t.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("gives each role skill only to a role of the squad", () => {
    expect(Object.keys(t.roleSkills ?? {}).sort()).toEqual(Object.keys(kitvietSquad("kv_legal").roleSkills).sort());
  });

  it("writes a charter with a mission, responsibilities, outputs and the boundary sentence", () => {
    const c = t.charter;
    expect(c.mission.trim()).not.toBe("");
    expect(c.responsibilities.length).toBeGreaterThan(0);
    expect(new Set(c.responsibilities.map((r) => r.key)).size).toBe(c.responsibilities.length);
    for (const r of c.responsibilities) expect(r.text.trim(), r.key).not.toBe("");
    expect(c.outputs.length).toBeGreaterThan(0);
    for (const o of c.outputs) {
      expect(CHARTER_OUTPUT_KINDS, o.key).toContain(o.kind);
      expect(o.cadenceHint.trim(), o.key).not.toBe("");
    }
    expect(c.boundaries).toContain("Nothing leaves the workspace");
    expect(c.escalation).toContain("[CẦN LUẬT SƯ]");
    expect(c.intake.trim()).not.toBe("");
  });

  it("runs one routine that opens a dated issue weekly and starts paused", () => {
    expect(t.routines).toHaveLength(1);
    const routine = t.routines[0]!;
    expect(routine.title).toBe("Rà hiệu lực sổ quy định");
    expect(routine.issueTitleTemplate).toContain("{{date}}");
    expect(routine.cron).toBe("0 8 * * 1");
    expect(routine.startPaused).toBe(true);
    expect(routine.description.trim()).not.toBe("");
  });
});

describe("legal role data", () => {
  it("takes each role's name and kind from the generated data", () => {
    const roles = new Map(kitvietSquad("kv_legal").roles.map((role) => [role.key, role] as const));
    for (const key of LEGAL_ROLE_KEYS) {
      expect(roles.get(key)?.name, key).toBe(LEGAL_ROLES[key].name);
      expect(roles.get(key)?.kind, key).toBe(LEGAL_ROLES[key].agentTemplate);
    }
    expect(LEGAL_ROLES.legal_lead.agentTemplate).toBe("kv_lead");
  });
});
