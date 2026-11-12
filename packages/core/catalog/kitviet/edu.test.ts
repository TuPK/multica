// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KV_LEAD_NOTES, composeKvAgentInstructions, type KvRoleKind } from "../../squads/kitviet-agent-instructions";
import { KITVIET_P2_FLOOR_NAME_LIST, KITVIET_P2_FLOOR_NAMES } from "../../squads/kitviet-floors";
import { loadKitvietRoleInstructions } from "../../squads/kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../../squads/kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../generated/kitviet/edu";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../generated/kitviet/common";
import { SQUAD_ROLES, SQUAD_TEMPLATES, type SquadRoleKey } from "../../squads/templates";

// The squad is catalog data (catalog/squads/kv_education), assembled into SQUAD_TEMPLATES and SQUAD_ROLES.
const EDU_CHARTER = SQUAD_TEMPLATES.find((t) => t.id === "kv_education")!.charter;
const EDU_ROLE_KEYS = SQUAD_TEMPLATES.find((t) => t.id === "kv_education")!.roles;
const EDU_ROLES = Object.fromEntries(SQUAD_TEMPLATES.find((t) => t.id === "kv_education")!.roles.map((key) => [key, SQUAD_ROLES[key]])) as typeof SQUAD_ROLES;
type EduRoleKey = SquadRoleKey;
const template = SQUAD_TEMPLATES.find((t) => t.id === "kv_education")!;

const KITVIET_SQUAD = kitvietSquad("kv_education");
const generatedRole = (key: string) => {
  const found = findKitvietRole(key);
  if (!found) throw new Error(`no generated role ${key}`);
  return found.role;
};
async function instructionOf(key: string): Promise<string> {
  const text = await loadKitvietRoleInstructions(key);
  if (text === undefined) throw new Error(`no Kit Viet instructions for ${key}`);
  return text;
}

const PACK_SKILLS = new Set([...COMMON_SKILLS, ...KITVIET_BUILTIN_SKILLS].map((skill) => skill.name));
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];

describe("Education Desk template (kv_education)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_education");
    expect(template.id).toBe("kv_education");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("Education Desk (Giáo dục)");
    expect(template.description.trim()).not.toBe("");
  });

  it("describes the squad with its test and answer key keywords", () => {
    expect(template.description).toContain("đề kiểm tra định kỳ");
    expect(template.description).toContain("answer key");
  });

  it("has the six desks of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...EDU_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = EDU_ROLE_KEYS.filter((key) => EDU_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
  });

  it("keeps at most eight members besides the lead", () => {
    const members = template.roles.filter((key) => key !== template.leader);

    expect(members.length).toBeLessThanOrEqual(8);
    expect(members).toHaveLength(5);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of EDU_ROLE_KEYS) {
      const definition = EDU_ROLES[key];
      expect(definition.key, key).toBe(key);
      expect(definition.description.trim(), key).not.toBe("");
      expect([...definition.description].length, key).toBeLessThanOrEqual(255);
      expect(definition.role.trim(), key).not.toBe("");
      expect(names.has(definition.name), definition.name).toBe(false);
      names.add(definition.name);
      expect(definition.name).toBe(generatedRole(key).name);
      expect(AGENT_INSTRUCTION_TEMPLATES.map((t) => t.id)).toContain(definition.agentTemplate);
    }
  });

  it("seats the squad on the Education floor from the P2 floor list", () => {
    expect(template.floorName).toBe(KITVIET_P2_FLOOR_NAMES.edu);
    expect(template.floorName).toBe("Education");
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches the pipeline day-chuyen-de-kiem-tra-dinh-ky as a core skill", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-de-kiem-tra-dinh-ky");
  });

  it("names only skills that exist in the Kit Viet pack, with the squad's own prefix", () => {
    const named = [
      ...template.builtinSkills,
      ...(template.leaderSkills ?? []),
      ...Object.values(template.roleSkills ?? {}).flat(),
    ];

    expect(named.length).toBeGreaterThan(0);
    for (const name of named) {
      expect(PACK_SKILLS.has(name), name).toBe(true);
      expect(name, name).toMatch(/^kv--[a-z0-9-]+$/);
    }
  });

  it("attaches role skills only to roles of this squad, at most six each, without repeats", () => {
    for (const [key, skills] of Object.entries(template.roleSkills ?? {})) {
      expect(template.roles, key).toContain(key);
      expect(skills.length, key).toBeLessThanOrEqual(6);
      expect(new Set(skills).size, key).toBe(skills.length);
    }
  });

  it("gives the test drafter its matrix, question bank and grading skills", () => {
    expect(template.roleSkills?.edu_test_drafter).toEqual([
      "kv--ma-tran-dac-ta-de-kiem-tra",
      "kv--ngan-hang-cau-hoi-trac-nghiem",
      "kv--rubric-cham-bai-hoc-sinh",
    ]);
  });

  it("gives the lead the two shared leader skills", () => {
    expect(template.leaderSkills).toEqual(KITVIET_SQUAD.leaderSkills);
    expect(template.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("writes a charter with a mission, responsibilities, outputs, boundaries, escalation and intake", () => {
    const charter = template.charter;
    const keys = charter.responsibilities.map((item) => item.key);

    expect(charter).toBe(EDU_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
    expect(charter.boundaries).toContain("Student records are not processed here");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("đề kiểm tra");
    expect(charter.intake).toContain("đáp án");
  });

  it("gives the lead the squad's rules: the answer key stays apart, student records blocked, nothing sent", () => {
    const header = generatedRole("edu_lead").header;
    expect(template.instructions.startsWith(header)).toBe(true);
    expect(template.instructions).toContain(KV_LEAD_NOTES.kv_education ?? "");
    expect(KV_LEAD_NOTES.kv_education ?? "").toContain("does not process student records");
    expect(KV_LEAD_NOTES.kv_education ?? "").toContain("never go into the student copy");
    expect(KV_LEAD_NOTES.kv_education ?? "").toContain("Nothing leaves the workspace");
    expect(template.instructions).toContain("Hồ sơ đơn vị");
  });
});

describe("Education Desk agent instructions", () => {
  it("has an instruction text for every role and no other", async () => {
    const texts = await Promise.all(EDU_ROLE_KEYS.map((key) => instructionOf(key)));
    expect(texts).toHaveLength(EDU_ROLE_KEYS.length);
  });

  it("starts each role with its header and its kind text, with one opener", async () => {
    for (const key of EDU_ROLE_KEYS) {
      if (key === "edu_lead") continue;
      const generated = generatedRole(key);
      const text = await instructionOf(key);

      expect(text.startsWith(generated.header), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain(composeKvAgentInstructions({ kind: generated.kind as KvRoleKind }).split("\n\n")[1]);
    }
  });

  it("adds each member's trade body from the vendored agent, and the lead none", async () => {
    for (const key of EDU_ROLE_KEYS) {
      if (key === "edu_lead") continue;
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;
      const text = await instructionOf(key);

      expect(body, key).toBeTruthy();
      expect(text.includes(body!), key).toBe(true);
    }
  });

  it("gives each gate exactly one VERDICT line in its body", async () => {
    for (const key of EDU_ROLE_KEYS) {
      if (EDU_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("keeps the two gates as gates: they write nothing and reach no network", () => {
    for (const key of ["edu_answer_gate", "edu_student_gate"] as EduRoleKey[]) {
      expect(EDU_ROLES[key].agentTemplate, key).toBe("kv_gate");
      expect(template.roleSkills?.[key], key).toBeUndefined();
    }
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(EDU_ROLE_KEYS.map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
