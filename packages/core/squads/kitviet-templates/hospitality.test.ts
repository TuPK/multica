// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KV_LEAD_NOTES, composeKvAgentInstructions, type KvRoleKind } from "../kitviet-agent-instructions";
import { KITVIET_P2_FLOOR_NAME_LIST, KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../kitviet-skills-data/hospitality";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../kitviet-skills-data/common";
import {
  HOSPITALITY_CHARTER,
  HOSPITALITY_ROLE_KEYS,
  HOSPITALITY_ROLES,
  type HospitalityRoleKey,
  KV_HOSPITALITY_TEMPLATE as template,
} from "./hospitality";

const KITVIET_SQUAD = kitvietSquad("kv_hospitality");
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

describe("Hospitality Ops template (kv_hospitality)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_hospitality");
    expect(template.id).toBe("kv_hospitality");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("Hospitality Ops (Du lịch, khách sạn)");
    expect(template.description.trim()).not.toBe("");
    expect(template.description).not.toContain("\n");
  });

  it("states that it books, charges and sends nothing", () => {
    expect(template.description).toContain("Books, charges and sends nothing");
    expect(template.description).toContain("group tour itineraries and quotes");
  });

  it("has the eight roles of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...HOSPITALITY_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = HOSPITALITY_ROLE_KEYS.filter((key) => HOSPITALITY_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
    expect(HOSPITALITY_ROLES[template.leader as HospitalityRoleKey].agentTemplate).toBe("kv_lead");
  });

  it("keeps at most eight members besides the lead", () => {
    const members = template.roles.filter((key) => key !== template.leader);

    expect(members.length).toBeLessThanOrEqual(8);
    expect(members).toHaveLength(7);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of HOSPITALITY_ROLE_KEYS) {
      const definition = HOSPITALITY_ROLES[key];
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

  it("seats the squad on the Hospitality floor, which the P2 floor list names", () => {
    expect(template.floorName).toBe("Hospitality");
    expect(template.floorName).toBe(KITVIET_P2_FLOOR_NAMES.hospitality);
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches the pipeline day-chuyen-ho-so-tour-doan as a core skill", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-ho-so-tour-doan");
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

  it("writes a charter that states the no-booking, no-promise and nothing-leaves rules", () => {
    const charter = template.charter;
    const keys = charter.responsibilities.map((item) => item.key);

    expect(charter).toBe(HOSPITALITY_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);

    expect(charter.boundaries).toContain("Nothing is booked, charged or sent to guests from the workspace");
    expect(charter.boundaries).toContain("The manager approves rates, refunds, deposits");
    expect(charter.escalation).toContain("never written as a promise");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("báo giá");
    expect(charter.intake).toContain("xác nhận đặt phòng");
    expect(charter.intake).toContain("khiếu nại");
  });

  it("makes the lead keep the promise rule, the guest-data rule and the nothing-sent rule", async () => {
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(template.instructions).toContain(KV_LEAD_NOTES.kv_hospitality ?? "");
    expect(template.instructions).toContain("Guest promises");
    expect(template.instructions).toContain("Guest data");
    expect(template.instructions).toContain("CHẶN");
    expect(template.instructions).toContain("Nothing leaves the workspace");
    expect(template.instructions).toBe(
      composeKvAgentInstructions({
        kind: "kv_lead" as KvRoleKind,
        header: generatedRole("hospitality_lead").header,
        notes: KV_LEAD_NOTES.kv_hospitality ?? "",
      }),
    );
  });
});

describe("Hospitality Ops agent instructions", () => {
  it("has an instruction text for every role and no other", async () => {
    const texts = await Promise.all(HOSPITALITY_ROLE_KEYS.map((key) => instructionOf(key)));
    expect(texts).toHaveLength(HOSPITALITY_ROLE_KEYS.length);
  });

  it("starts each member role with its header and its kind text, with one opener", async () => {
    for (const key of HOSPITALITY_ROLE_KEYS.filter((k) => k !== template.leader)) {
      const generated = generatedRole(key);
      const text = await instructionOf(key);

      expect(text.startsWith(generated.header), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain(composeKvAgentInstructions({ kind: generated.kind as KvRoleKind }).split("\n\n")[1]);
    }
  });

  it("opens the lead with its header and one opener", () => {
    const generated = generatedRole(template.leader);

    expect(template.instructions.startsWith(generated.header)).toBe(true);
    expect(template.instructions.match(/^You are /gm)?.length ?? 0).toBe(1);
  });

  it("adds each member's trade body from the vendored agent, and the lead none", async () => {
    for (const key of HOSPITALITY_ROLE_KEYS) {
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;

      if (key === template.leader) {
        expect(body).toBeUndefined();
      } else {
        const text = await instructionOf(key);
        expect(body, key).toBeTruthy();
        expect(text.includes(body!), key).toBe(true);
      }
    }
  });

  it("gives each gate exactly one VERDICT line in its body", async () => {
    for (const key of HOSPITALITY_ROLE_KEYS) {
      if (HOSPITALITY_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("gives each gate the severity vocabulary of the pack, with no old word", async () => {
    for (const key of HOSPITALITY_ROLE_KEYS) {
      if (HOSPITALITY_ROLES[key].agentTemplate !== "kv_gate") continue;
      const text = await instructionOf(key);
      expect(text, key).toContain("Chặn");
      expect(text, key).toContain("Góp ý");
      expect(text.includes("Nên sửa") || text.includes("Cần giấy tờ"), key).toBe(true);
      for (const word of ["Lưu ý", "Nặng", "Vừa", "Nhẹ", "Phải sửa"]) expect(text.includes(word), `${key}: ${word}`).toBe(false);
    }
  });

  it("blocks the quote and the promise checks on a claim with no source (Cần giấy tờ blocks)", async () => {
    for (const key of ["hospitality_quote_gate", "hospitality_promise_gate"]) {
      const text = await instructionOf(key);
      expect(text, key).toContain("**Cần giấy tờ**");
      expect(text, key).toContain("CHẶN khi có lỗi mức Chặn hoặc Cần giấy tờ.");
    }
  });

  it("keeps the two price and promise checks as gates with no write tool", async () => {
    for (const key of ["hospitality_quote_gate", "hospitality_promise_gate"]) {
      expect(generatedRole(key).kind, key).toBe("kv_gate");
      expect(await instructionOf(key), key).toContain("Không có công cụ ghi");
    }
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(HOSPITALITY_ROLE_KEYS.filter((k) => k !== template.leader).map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
