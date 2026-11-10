// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { COMPANY_FLOOR_NAMES } from "../company-template";
import { KV_LEAD_NOTES } from "../kitviet-agent-instructions";
import { kitvietSquad } from "../kitviet-squad-roles";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import * as commonPack from "../kitviet-skills-data/common";
import * as academicPack from "../kitviet-skills-data/academic";
import { ACADEMIC_ROLE_KEYS, ACADEMIC_ROLES, KV_ACADEMIC_TEMPLATE } from "./academic";

const t = KV_ACADEMIC_TEMPLATE;
const DESK_LIMIT = 9; // one lead and eight members (spec 5.1)
const CHARTER_OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];
const PACK_SKILL_NAMES = new Set(
  [...academicPack.KITVIET_BUILTIN_SKILLS, ...commonPack.KITVIET_BUILTIN_SKILLS].map((skill) => skill.name),
);

describe("KV_ACADEMIC_TEMPLATE", () => {
  it("names the squad, its floor and its template id", () => {
    expect(t.id).toBe("kv_academic");
    expect(t.name).toBe("Academic Evidence Desk (Học thuật, nghiên cứu)");
    expect(t.description.trim()).not.toBe("");
    expect(t.floorName).toBe("Research");
    expect(Object.values(COMPANY_FLOOR_NAMES)).toContain(t.floorName);
    expect(t.workflow).toBeNull();
  });

  it("lists every role, the same roles as the generated pack, with the leader first", () => {
    expect(t.roles).toEqual([...ACADEMIC_ROLE_KEYS]);
    expect([...t.roles].sort()).toEqual([...kitvietSquad("kv_academic").roles.map((role) => role.key)].sort());
    expect(t.roles[0]).toBe(t.leader);
    expect(new Set(t.roles).size).toBe(t.roles.length);
    for (const key of ACADEMIC_ROLE_KEYS) expect(ACADEMIC_ROLES[key].key, key).toBe(key);
  });

  it("has exactly one lead, and its roles fit the desk limit", () => {
    const leads = ACADEMIC_ROLE_KEYS.filter((key) => ACADEMIC_ROLES[key].agentTemplate === "kv_lead");
    expect(leads).toEqual([t.leader]);
    expect(t.roles.length).toBeLessThanOrEqual(DESK_LIMIT);
    expect(t.roles.length - 1).toBeLessThanOrEqual(8);
  });

  it("builds every role from an existing agent instruction template with a short description", () => {
    const ids = new Set(AGENT_INSTRUCTION_TEMPLATES.map((template) => template.id));
    const names = new Set<string>();
    for (const key of ACADEMIC_ROLE_KEYS) {
      const role = ACADEMIC_ROLES[key];
      expect(ids.has(role.agentTemplate), key).toBe(true);
      expect([...role.description].length, key).toBeLessThanOrEqual(255);
      expect(role.name.trim(), key).not.toBe("");
      expect(names.has(role.name), role.name).toBe(false);
      names.add(role.name);
    }
  });

  it("makes the citation gate a gate, and the web-search roles collectors", () => {
    expect(ACADEMIC_ROLES.acad_citation_gate.agentTemplate).toBe("kv_gate");
    expect(ACADEMIC_ROLES.acad_integrity_gate.agentTemplate).toBe("kv_gate");
    expect(ACADEMIC_ROLES.acad_finder.agentTemplate).toBe("kv_collector");
  });

  it("gives each agent its header, its kind text and, except the lead, its body", async () => {
    for (const key of ACADEMIC_ROLE_KEYS) {
      const text = await loadKitvietRoleInstructions(key);
      if (text === undefined) throw new Error(`no Kit Viet instructions for ${key}`);
      expect(text.startsWith("You are the "), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain("in an Academic Evidence Desk (Học thuật, nghiên cứu) squad,");
      if (key !== t.leader) expect(text.length, key).toBeGreaterThan(500);
      if (ACADEMIC_ROLES[key].agentTemplate === "kv_gate") expect(text, key).toContain("VERDICT:");
    }
    expect(t.instructions).toBe(await loadKitvietRoleInstructions("acad_lead"));
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

  it("gives the citation gate the citation-check skill", () => {
    expect(t.roleSkills?.acad_citation_gate).toContain("kv--kiem-tra-trich-dan-co-that");
  });

  it("keeps the core skills and the leader skills in the generated pack", () => {
    expect(t.builtinSkills).toEqual(kitvietSquad("kv_academic").builtinSkills);
    expect(t.leaderSkills).toEqual(kitvietSquad("kv_academic").leaderSkills);
    expect(t.builtinSkills).toContain("kv--delivery-core");
    expect(t.builtinSkills).toContain("kv--day-chuyen-viet-chuong-luan-van");
    expect(t.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("caps each role's own skills at six and gives them only to roles of the squad", () => {
    for (const [key, skills] of Object.entries(t.roleSkills ?? {})) {
      expect(skills?.length ?? 0, key).toBeLessThanOrEqual(6);
    }
    expect(Object.keys(t.roleSkills ?? {}).sort()).toEqual(Object.keys(kitvietSquad("kv_academic").roleSkills).sort());
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
    expect(c.boundaries).toContain("Writes no graded work");
    expect(c.boundaries).toContain("[chưa xác minh]");
    expect(c.escalation).toContain("[CẦN NGƯỜI HƯỚNG DẪN]");
    expect(c.intake.trim()).not.toBe("");
  });

  it("carries the Vietnamese intake keywords of the trade", () => {
    for (const keyword of ["luận văn", "trích dẫn", "tổng quan tài liệu"]) {
      expect(t.charter.intake, keyword).toContain(keyword);
    }
  });

  it("runs no routine: the desk works per request", () => {
    expect(t.routines).toEqual([]);
  });

  it("states the integrity boundary and the citation rule in the lead's notes", () => {
    expect(KV_LEAD_NOTES.kv_academic ?? "").toContain("Integrity boundary");
    expect(KV_LEAD_NOTES.kv_academic ?? "").toContain("[chưa xác minh]");
    expect(KV_LEAD_NOTES.kv_academic ?? "").toContain("human stop 1");
  });
});

describe("academic role data", () => {
  it("takes each role's name and kind from the generated data", () => {
    const roles = new Map(kitvietSquad("kv_academic").roles.map((role) => [role.key, role] as const));
    for (const key of ACADEMIC_ROLE_KEYS) {
      expect(roles.get(key)?.name, key).toBe(ACADEMIC_ROLES[key].name);
      expect(roles.get(key)?.kind, key).toBe(ACADEMIC_ROLES[key].agentTemplate);
    }
    expect(ACADEMIC_ROLES.acad_lead.agentTemplate).toBe("kv_lead");
  });
});
