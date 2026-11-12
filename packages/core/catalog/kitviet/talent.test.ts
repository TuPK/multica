// @vitest-environment node
import { describe, expect, it } from "vitest";
import { COMPANY_FLOOR_NAMES } from "../../squads/company-template";
import { KV_LEAD_NOTES } from "../../squads/kitviet-agent-instructions";
import { loadKitvietRoleInstructions } from "../../squads/kitviet-packs";
import { KITVIET_BUILTIN_SKILLS as COMMON_PACK_SKILLS } from "../generated/kitviet/common";
import { KITVIET_BUILTIN_SKILLS as TALENT_PACK_SKILLS } from "../generated/kitviet/talent";
import { SQUAD_ROLES, SQUAD_TEMPLATES } from "../../squads/templates";

// The squad is catalog data (catalog/squads/kv_talent), assembled into SQUAD_TEMPLATES and SQUAD_ROLES.
const KV_TALENT_ROLE_KEYS = SQUAD_TEMPLATES.find((t) => t.id === "kv_talent")!.roles;
const KV_TALENT_ROLES = Object.fromEntries(SQUAD_TEMPLATES.find((t) => t.id === "kv_talent")!.roles.map((key) => [key, SQUAD_ROLES[key]])) as typeof SQUAD_ROLES;
const KV_TALENT_TEMPLATE = SQUAD_TEMPLATES.find((t) => t.id === "kv_talent")!;

const packSkillNames = new Set(
  [...TALENT_PACK_SKILLS, ...COMMON_PACK_SKILLS].map((skill) => skill.name),
);
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];
const CRON_FIELD = /^[\d*/,-]+$/;

describe("Talent Acquisition (kv_talent) squad template", () => {
  const t = KV_TALENT_TEMPLATE;

  it("has the id, name and floor of the Kit Viet spec", () => {
    expect(t.id).toBe("kv_talent");
    expect(t.name).toBe("Talent Acquisition (Tuyển dụng)");
    expect(t.floorName).toBe("Governance");
  });

  it("has a one-line description", () => {
    expect(t.description.trim()).not.toBe("");
    expect(t.description).not.toContain("\n");
  });

  it("has every role of the squad, each defined once", () => {
    expect(t.roles).toEqual([...KV_TALENT_ROLE_KEYS]);
    expect(new Set(t.roles).size).toBe(t.roles.length);
    expect(Object.keys(KV_TALENT_ROLES).sort()).toEqual([...KV_TALENT_ROLE_KEYS].sort());
  });

  it("uses only talent_ role keys", () => {
    for (const key of KV_TALENT_ROLE_KEYS) expect(key.startsWith("talent_"), key).toBe(true);
  });

  it("has exactly one lead, and it is the leader", () => {
    const leads = KV_TALENT_ROLE_KEYS.filter((key) => KV_TALENT_ROLES[key].agentTemplate === "kv_lead");
    expect(leads).toEqual([t.leader]);
    expect(t.roles[0]).toBe(t.leader);
    for (const key of KV_TALENT_ROLE_KEYS.filter((k) => k !== t.leader)) {
      expect(KV_TALENT_ROLES[key].agentTemplate, key).not.toBe("kv_lead");
    }
  });

  it("keeps the squad within the desk limit: at most 8 members besides the lead", () => {
    const members = KV_TALENT_ROLE_KEYS.filter((key) => key !== t.leader);
    expect(members.length).toBeLessThanOrEqual(8);
    expect(t.roles.length).toBeLessThanOrEqual(9);
  });

  it("seats the squad on a default floor", () => {
    expect(Object.values(COMPANY_FLOOR_NAMES)).toContain(t.floorName);
  });

  it("gives each role a name, an English description within the limit, and a roster line", () => {
    const names = new Set<string>();
    for (const key of KV_TALENT_ROLE_KEYS) {
      const role = KV_TALENT_ROLES[key];
      expect(role.key, key).toBe(key);
      expect(role.name.trim(), key).not.toBe("");
      expect(names.has(role.name), role.name).toBe(false);
      names.add(role.name);
      expect(role.description.trim(), key).not.toBe("");
      expect([...role.description].length, key).toBeLessThanOrEqual(255);
      expect(role.role.trim(), key).not.toBe("");
    }
  });

  it("keeps the type prefix out of the roster lines", () => {
    for (const key of KV_TALENT_ROLE_KEYS) {
      expect(KV_TALENT_ROLES[key].role, key).not.toMatch(/^(Thu thập|Thực thi|Gác cổng)/);
    }
  });

  it("builds each role's instructions from its header, its kind text and its body, with one opener", async () => {
    for (const key of KV_TALENT_ROLE_KEYS) {
      const text = await loadKitvietRoleInstructions(key);
      if (text === undefined) throw new Error(`no Kit Viet instructions for ${key}`);
      expect(text.startsWith(`You are the ${KV_TALENT_ROLES[key].name} in a Talent Acquisition`), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain("unattended");
    }
  });

  it("gives the lead instructions with the squad notes and the lead's skills", () => {
    expect(t.instructions).toContain(KV_LEAD_NOTES.kv_talent ?? "");
    expect(t.instructions).toContain("kv--squad-lead");
    expect(t.instructions.startsWith("You are the Talent Lead in a Talent Acquisition")).toBe(true);
  });

  it("has no workflow", () => {
    expect(t.workflow).toBeNull();
  });

  it("names only built-in skills that exist in the pack", () => {
    const names = [...t.builtinSkills, ...(t.leaderSkills ?? [])];
    for (const roleNames of Object.values(t.roleSkills ?? {})) names.push(...(roleNames ?? []));

    expect(new Set(t.builtinSkills).size).toBe(t.builtinSkills.length);
    for (const name of names) {
      expect(packSkillNames.has(name), name).toBe(true);
    }
  });

  it("does not attach a skill another squad ships (soan-don-tu belongs to Office Admin)", () => {
    const attached = [...t.builtinSkills, ...Object.values(t.roleSkills ?? {}).flat()];
    expect(attached).not.toContain("kv--soan-don-tu");
  });

  it("gives only roles of this squad their extra skills, and each role at most six", () => {
    for (const [key, skills] of Object.entries(t.roleSkills ?? {})) {
      expect(KV_TALENT_ROLE_KEYS, key).toContain(key);
      expect(skills?.length ?? 0, key).toBeLessThanOrEqual(6);
    }
  });

  it("holds the core skills and the lead skills of the pack", () => {
    expect(t.builtinSkills).toContain("kv--day-chuyen-tuyen-dung");
    expect(t.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("has a valid charter", () => {
    const c = t.charter;
    expect(c.mission.trim()).not.toBe("");
    expect(c.responsibilities.length).toBeGreaterThan(0);
    expect(new Set(c.responsibilities.map((r) => r.key)).size).toBe(c.responsibilities.length);
    for (const r of c.responsibilities) expect(r.text.trim(), r.key).not.toBe("");
    expect(c.outputs.length).toBeGreaterThan(0);
    for (const output of c.outputs) {
      expect(OUTPUT_KINDS, output.key).toContain(output.kind);
      expect(output.cadenceHint.trim(), output.key).not.toBe("");
    }
    expect(c.boundaries).toContain("Nothing leaves the workspace");
    expect(c.boundaries).toContain("Does not contact candidates");
    expect(c.escalation.trim()).not.toBe("");
    expect(c.intake.trim()).not.toBe("");
  });

  it("has no routine: the trade does not need one", () => {
    expect(t.routines).toEqual([]);
  });

  it("gives every routine a five-field cron and starts it paused", () => {
    for (const routine of t.routines) {
      const fields = routine.cron.trim().split(/\s+/);
      expect(fields.length, routine.title).toBe(5);
      for (const field of fields) expect(field, routine.title).toMatch(CRON_FIELD);
      expect(routine.startPaused, routine.title).toBe(true);
    }
  });
});
