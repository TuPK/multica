// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KITVIET_P2_FLOOR_NAME_LIST } from "../kitviet-floors";
import { KV_LEAD_NOTES, composeKvAgentInstructions, type KvRoleKind } from "../kitviet-agent-instructions";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../kitviet-skills-data/trade";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../kitviet-skills-data/common";
import { KV_TRADE_TEMPLATE as template, TRADE_CHARTER, TRADE_ROLE_KEYS, TRADE_ROLES, type TradeRoleKey } from "./trade";

const KITVIET_SQUAD = kitvietSquad("kv_trade");
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

describe("Import & Export Desk template (kv_trade)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_trade");
    expect(template.id).toBe("kv_trade");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("Import & Export Desk (Xuất nhập khẩu)");
    expect(template.description.trim()).not.toBe("");
  });

  it("describes the squad with its shipment and customs keywords", () => {
    expect(template.description).toContain("shipment");
    expect(template.description).toContain("customs declaration");
  });

  it("has the seven roles of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...TRADE_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(template.roles).toContain(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = TRADE_ROLE_KEYS.filter((key) => TRADE_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
    expect(TRADE_ROLES[template.leader as TradeRoleKey].agentTemplate).toBe("kv_lead");
  });

  it("keeps at most eight members besides the lead (six here)", () => {
    const members = template.roles.filter((key) => key !== template.leader);

    expect(members.length).toBeLessThanOrEqual(8);
    expect(members).toHaveLength(6);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of TRADE_ROLE_KEYS) {
      const definition = TRADE_ROLES[key];
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
    expect(template.floorName).toBe("Import & Export");
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches the pipeline day-chuyen-ho-so-lo-hang as a core skill", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-ho-so-lo-hang");
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

    expect(charter).toBe(TRADE_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("tờ khai");
    expect(charter.intake).toContain("mã HS");
  });

  it("makes the lead follow the squad's rules: both gates, the rate stop, nothing declared", async () => {
    expect(template.instructions).toBe(await instructionOf("trade_lead"));
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(KV_LEAD_NOTES.kv_trade ?? "").toContain("Hồ sơ đơn vị");
    expect(KV_LEAD_NOTES.kv_trade ?? "").toContain("trade_docs_gate and trade_customs_gate both return QUA");
    expect(KV_LEAD_NOTES.kv_trade ?? "").toContain("[cần tra biểu thuế]");
    expect(KV_LEAD_NOTES.kv_trade ?? "").toContain("Nothing is declared to customs");
  });
});

describe("Import & Export Desk agent instructions", () => {
  it("has an instruction text for every role and no other", async () => {
    const texts = await Promise.all(TRADE_ROLE_KEYS.map((key) => instructionOf(key)));
    expect(texts).toHaveLength(TRADE_ROLE_KEYS.length);
  });

  it("starts each role with its header and its kind text, with one opener", async () => {
    for (const key of TRADE_ROLE_KEYS) {
      const generated = generatedRole(key);
      const text = await instructionOf(key);

      expect(text.startsWith(generated.header), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain(composeKvAgentInstructions({ kind: generated.kind as KvRoleKind }).split("\n\n")[1]);
    }
  });

  it("adds each member's trade body from the vendored agent, and the lead none", async () => {
    for (const key of TRADE_ROLE_KEYS) {
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;
      const text = await instructionOf(key);

      if (key === "trade_lead") {
        expect(body).toBeUndefined();
        expect(text).toBe(template.instructions);
      } else {
        expect(body, key).toBeTruthy();
        expect(text.includes(body!), key).toBe(true);
      }
    }
  });

  it("gives each gate exactly one VERDICT line in its body", async () => {
    for (const key of TRADE_ROLE_KEYS) {
      if (TRADE_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(TRADE_ROLE_KEYS.map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
