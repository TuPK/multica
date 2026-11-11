// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KV_LEAD_NOTES, composeKvAgentInstructions, type KvRoleKind } from "../kitviet-agent-instructions";
import { KITVIET_P2_FLOOR_NAME_LIST } from "../kitviet-floors";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../kitviet-skills-data/fnb";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../kitviet-skills-data/common";
import { FNB_CHARTER, FNB_ROLE_KEYS, FNB_ROLES, KV_FNB_TEMPLATE as template, type FnbRoleKey } from "./fnb";

const KITVIET_SQUAD = kitvietSquad("kv_fnb");
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

describe("F&B Ops template (kv_fnb)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_fnb");
    expect(template.id).toBe("kv_fnb");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("F&B Ops (Nhà hàng, quán ăn)");
    expect(template.description.trim()).not.toBe("");
  });

  it("describes the squad with its dish, allergen and food cost keywords", () => {
    expect(template.description).toContain("food cost");
    expect(template.description).toContain("allergen");
  });

  it("has the seven roles of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...FNB_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = FNB_ROLE_KEYS.filter((key) => FNB_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
    expect(FNB_ROLES[template.leader as FnbRoleKey].agentTemplate).toBe("kv_lead");
  });

  it("keeps at most eight members besides the lead", () => {
    const members = template.roles.filter((key) => key !== template.leader);

    expect(members.length).toBeLessThanOrEqual(8);
    expect(members).toHaveLength(6);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of FNB_ROLE_KEYS) {
      const definition = FNB_ROLES[key];
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

  it("seats the squad on its own P2 floor, named in KITVIET_P2_FLOOR_NAMES", () => {
    expect(template.floorName).toBe("F&B");
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches the pipeline day-chuyen-ra-mat-mon-moi as a core skill", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-ra-mat-mon-moi");
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

  it("gives the lead the two shared leader skills", () => {
    expect(template.leaderSkills).toEqual(KITVIET_SQUAD.leaderSkills);
    expect(template.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("writes a charter with a mission, responsibilities, outputs, boundaries, escalation and intake", () => {
    const charter = template.charter;
    const keys = charter.responsibilities.map((item) => item.key);

    expect(charter).toBe(FNB_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
    expect(charter.boundaries).toContain("does not state that a dish is free of an allergen");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("food cost");
    expect(charter.intake).toContain("dị ứng");
  });

  it("keeps the allergen and food cost gates as gates, one each, in the charter's own gates line", () => {
    const gates = FNB_ROLE_KEYS.filter((key) => FNB_ROLES[key].agentTemplate === "kv_gate");

    expect(gates).toEqual(["fnb_allergen_gate", "fnb_margin_gate"]);
    expect(FNB_CHARTER.responsibilities.find((item) => item.key === "gates")?.text).toContain("allergen");
  });

  it("makes the lead the one whose text the quickstart reads, with the squad's rules in its notes", async () => {
    expect(template.instructions).toBe(await instructionOf("fnb_lead"));
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(KV_LEAD_NOTES.kv_fnb ?? "").toContain("Stop 1");
    expect(KV_LEAD_NOTES.kv_fnb ?? "").toContain("The allergen gate is never skipped");
    expect(KV_LEAD_NOTES.kv_fnb ?? "").toContain("Nothing is printed, posted to a delivery app");
  });
});

describe("F&B Ops agent instructions", () => {
  it("has an instruction text for every role and no other", async () => {
    const texts = await Promise.all(FNB_ROLE_KEYS.map((key) => instructionOf(key)));
    expect(texts).toHaveLength(FNB_ROLE_KEYS.length);
  });

  it("starts each role with its header and its kind text, with one opener", async () => {
    for (const key of FNB_ROLE_KEYS) {
      const generated = generatedRole(key);
      const text = await instructionOf(key);

      expect(text.startsWith(generated.header), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain(composeKvAgentInstructions({ kind: generated.kind as KvRoleKind }).split("\n\n")[1]);
    }
  });

  it("adds each member's trade body from the vendored agent, and the lead none", async () => {
    for (const key of FNB_ROLE_KEYS) {
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;
      const text = await instructionOf(key);

      if (key === "fnb_lead") {
        expect(body).toBeUndefined();
        expect(text).toBe(template.instructions);
      } else {
        expect(body, key).toBeTruthy();
        expect(text.includes(body!), key).toBe(true);
      }
    }
  });

  it("gives each gate exactly one VERDICT line in its body", async () => {
    for (const key of FNB_ROLE_KEYS) {
      if (FNB_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("uses the unified severity scale in the gates, not the old per-file words", async () => {
    for (const key of ["fnb_allergen_gate", "fnb_margin_gate"] as const) {
      const text = await instructionOf(key);
      expect(text, key).toContain("Chặn");
      expect(text, key).toContain("Cần giấy tờ");
      expect(text, key).not.toMatch(/\*\*(Sửa|Lưu ý|Nặng|Vừa|Nhẹ|Chưa xác nhận)\*\*/);
    }
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(FNB_ROLE_KEYS.map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
