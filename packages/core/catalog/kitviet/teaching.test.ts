// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KITVIET_P2_FLOOR_NAME_LIST, KITVIET_P2_FLOOR_NAMES } from "../../squads/kitviet-floors";
import { loadKitvietRoleInstructions } from "../../squads/kitviet-packs";
import { composeKvAgentInstructions, KV_LEAD_NOTES, type KvRoleKind } from "../../squads/kitviet-agent-instructions";
import { findKitvietRole, kitvietSquad } from "../../squads/kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../generated/kitviet/teaching";
import { KITVIET_BUILTIN_SKILLS as EDU_SKILLS } from "../generated/kitviet/edu";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../generated/kitviet/common";
import { SQUAD_ROLES, SQUAD_TEMPLATES, type SquadRoleKey } from "../../squads/templates";

// The squad is catalog data (catalog/squads/kv_teaching), assembled into SQUAD_TEMPLATES and SQUAD_ROLES.
const template = SQUAD_TEMPLATES.find((t) => t.id === "kv_teaching")!;
const TEACHING_CHARTER = SQUAD_TEMPLATES.find((t) => t.id === "kv_teaching")!.charter;
const TEACHING_ROLE_KEYS = SQUAD_TEMPLATES.find((t) => t.id === "kv_teaching")!.roles;
const TEACHING_ROLES = Object.fromEntries(SQUAD_TEMPLATES.find((t) => t.id === "kv_teaching")!.roles.map((key) => [key, SQUAD_ROLES[key]])) as typeof SQUAD_ROLES;
type TeachingRoleKey = SquadRoleKey;

const KITVIET_SQUAD = kitvietSquad("kv_teaching");
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
const EDU_PACK_SKILLS = new Set(EDU_SKILLS.map((skill) => skill.name));
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];

// The sixteen skills Education Desk used to ship as deferred. Teaching Materials owns each one now.
const MOVED_FROM_EDU = [
  "ke-hoach-bai-day-gdpt",
  "day-hoc-phan-hoa-trinh-do",
  "ke-hoach-on-tap-cuoi-ky",
  "phieu-hoc-tap-nhom",
  "de-cuong-chuyen-de-bai-giang",
  "nghien-cuu-tu-lieu-bai-giang",
  "nhan-xet-hoc-ba",
  "hop-phu-huynh-lop",
  "tin-nhan-phu-huynh-zalo",
  "bao-cao-to-chuyen-mon",
  "sang-kien-kinh-nghiem",
  "ke-hoach-chu-nhiem-lop",
  "tro-choi-khoi-dong-on-tap",
  "bai-trinh-chieu-tu-giao-an",
  "de-cuong-nckh-su-pham-ung-dung",
  "phan-hoi-bai-viet-hoc-sinh",
].map((folder) => `kv--${folder}`);

describe("Teaching Materials template (kv_teaching)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_teaching");
    expect(template.id).toBe("kv_teaching");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("Teaching Materials (Tư liệu giảng dạy)");
    expect(template.description.trim()).not.toBe("");
  });

  it("describes the squad with its sources, class records and gates", () => {
    expect(template.description).toContain("teaching sources");
    expect(template.description).toContain("class records");
    expect(template.description).toContain("The squad sends nothing");
  });

  it("has the eight desks of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...TEACHING_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
    expect(template.roles.length).toBeLessThanOrEqual(9);
  });

  it("uses the tm_ prefix for every role key", () => {
    for (const key of TEACHING_ROLE_KEYS) expect(key, key).toMatch(/^tm_[a-z_]+$/);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = TEACHING_ROLE_KEYS.filter((key) => TEACHING_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of TEACHING_ROLE_KEYS) {
      const definition = TEACHING_ROLES[key];
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

  it("maps the four trade agents Education Desk left unmapped, with the lead and the handover", () => {
    const agents = KITVIET_AGENTS.map((agent) => agent.name);

    expect(agents).toContain("tm_sources");
    expect(agents).toContain("tm_records");
    expect(agents).toContain("tm_drafter");
    expect(agents).toContain("tm_source_gate");
    expect(agents).toContain("tm_handover");
    expect(TEACHING_ROLES.tm_sources.agentTemplate).toBe("kv_collector");
    expect(TEACHING_ROLES.tm_records.agentTemplate).toBe("kv_collector");
    expect(TEACHING_ROLES.tm_drafter.agentTemplate).toBe("kv_drafter");
    expect(TEACHING_ROLES.tm_source_gate.agentTemplate).toBe("kv_gate");
    expect(TEACHING_ROLES.tm_handover.agentTemplate).toBe("kv_handover");
  });

  it("gives the student-facing text a gate, and the figures a gate", () => {
    expect(TEACHING_ROLES.tm_student_gate.agentTemplate).toBe("kv_gate");
    expect(TEACHING_ROLES.tm_numbers_gate.agentTemplate).toBe("kv_gate");
  });

  it("seats the squad on the Education floor, the same floor as Education Desk", () => {
    expect(template.floorName).toBe(KITVIET_P2_FLOOR_NAMES.edu);
    expect(template.floorName).toBe("Education");
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches no pipeline skill: the squad is authored, and the pipeline belongs to Education Desk", () => {
    const builtin = template.builtinSkills.filter((name) => name.startsWith("kv--day-chuyen-"));
    expect(builtin).toEqual([]);
    expect(template.builtinSkills).toEqual(KITVIET_SQUAD.builtinSkills);
  });

  it("names only skills that exist in the Kit Viet pack, with the kv-- prefix", () => {
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

  it("gives the drafter the class-teacher, comment, parent-message and meeting skills it runs", () => {
    expect(template.roleSkills?.tm_drafter).toEqual([
      "kv--nhan-xet-hoc-ba",
      "kv--tin-nhan-phu-huynh-zalo",
      "kv--hop-phu-huynh-lop",
      "kv--ke-hoach-chu-nhiem-lop",
    ]);
  });

  it("gives the sources collector the materials skill it runs", () => {
    expect(template.roleSkills?.tm_sources).toEqual(["kv--nghien-cuu-tu-lieu-bai-giang"]);
  });

  it("gives no role skill to the records reader, the gates or the handover", () => {
    for (const key of ["tm_records", "tm_source_gate", "tm_student_gate", "tm_numbers_gate", "tm_handover"] as TeachingRoleKey[]) {
      expect(template.roleSkills?.[key], key).toBeUndefined();
    }
  });

  it("ships each of the sixteen former Education deferred skills, owned by this squad only", () => {
    const own = new Set(KITVIET_BUILTIN_SKILLS.map((skill) => skill.name));
    for (const name of MOVED_FROM_EDU) {
      expect(own.has(name), name).toBe(true);
      expect(EDU_PACK_SKILLS.has(name), name).toBe(false);
    }
  });

  it("gives the lead the two shared leader skills", () => {
    expect(template.leaderSkills).toEqual(KITVIET_SQUAD.leaderSkills);
    expect(template.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("writes a charter with a mission, responsibilities, outputs, boundaries, escalation and intake", () => {
    const charter = template.charter;
    const keys = charter.responsibilities.map((item) => item.key);

    expect(charter).toBe(TEACHING_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
    expect(charter.boundaries).toContain("Student records are not processed unless");
    expect(charter.escalation).toContain("stop type 1");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("tư liệu bài giảng");
    expect(charter.intake).toContain("tin nhắn phụ huynh");
  });

  it("gives the lead the squad's rules: student records stop at intake, codes only, nothing sent", () => {
    const header = generatedRole("tm_lead").header;
    const notes = KV_LEAD_NOTES.kv_teaching ?? "";
    expect(template.instructions.startsWith(header)).toBe(true);
    expect(template.instructions).toContain(notes);
    expect(notes).toContain("stop type 1 at intake");
    expect(notes).toContain("anonymised");
    expect(notes).toContain("never goes into an output");
    expect(notes).toContain("Nothing leaves the workspace");
    expect(notes).toContain("tm_student_gate");
    expect(template.instructions).toContain("Hồ sơ đơn vị");
  });

  it("builds the lead instructions from the kind text, the header and the notes only", () => {
    const expected = composeKvAgentInstructions({ kind: "kv_lead" as KvRoleKind, header: generatedRole("tm_lead").header, notes: KV_LEAD_NOTES.kv_teaching });
    expect(template.instructions).toBe(expected);
  });
});

describe("Teaching Materials agent instructions", () => {
  it("has an instruction text for every role and no other", async () => {
    const texts = await Promise.all(TEACHING_ROLE_KEYS.filter((key) => key !== "tm_lead").map((key) => instructionOf(key)));
    expect(texts).toHaveLength(TEACHING_ROLE_KEYS.length - 1);
  });

  it("starts each role with its header and its kind text, with one opener", async () => {
    for (const key of TEACHING_ROLE_KEYS) {
      if (key === "tm_lead") continue;
      const generated = generatedRole(key);
      const text = await instructionOf(key);

      expect(text.startsWith(generated.header), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain(composeKvAgentInstructions({ kind: generated.kind as KvRoleKind }).split("\n\n")[1]);
    }
  });

  it("adds each member's trade body from the vendored agent", async () => {
    for (const key of TEACHING_ROLE_KEYS) {
      if (key === "tm_lead") continue;
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;
      const text = await instructionOf(key);

      expect(body, key).toBeTruthy();
      expect(text.includes(body!), key).toBe(true);
    }
  });

  it("gives each gate exactly one VERDICT line in its body", async () => {
    for (const key of TEACHING_ROLE_KEYS) {
      if (TEACHING_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("blocks on Chặn or Cần giấy tờ in the two gates that guard sources and student data", async () => {
    for (const key of ["tm_source_gate", "tm_student_gate"] as TeachingRoleKey[]) {
      const text = await instructionOf(key);
      expect(text, key).toContain("Chặn");
      expect(text, key).toContain("Cần giấy tờ");
      expect(text, key).toContain("QUA");
    }
  });

  it("keeps the gates as gates: they write nothing and reach no network", () => {
    for (const key of ["tm_source_gate", "tm_student_gate", "tm_numbers_gate"] as TeachingRoleKey[]) {
      expect(TEACHING_ROLES[key].agentTemplate, key).toBe("kv_gate");
      expect(template.roleSkills?.[key], key).toBeUndefined();
    }
  });

  it("keeps student names and sensitive notes out of the records reader's output", async () => {
    const text = await instructionOf("tm_records");
    expect(text).toContain("Không in họ tên đầy đủ");
    expect(text).toContain("Không chuyển ghi chú hoàn cảnh, sức khỏe, kỷ luật");
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(TEACHING_ROLE_KEYS.filter((key) => key !== "tm_lead").map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
