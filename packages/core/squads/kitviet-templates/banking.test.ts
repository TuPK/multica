// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KV_LEAD_NOTES, composeKvAgentInstructions, type KvRoleKind } from "../kitviet-agent-instructions";
import { KITVIET_P2_FLOOR_NAME_LIST, KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../kitviet-skills-data/banking";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../kitviet-skills-data/common";
import {
  KV_BANKING_CHARTER,
  KV_BANKING_ROLE_KEYS,
  KV_BANKING_ROLES,
  KV_BANKING_TEMPLATE as template,
  type KvBankingRoleKey,
} from "./banking";

const KITVIET_SQUAD = kitvietSquad("kv_banking");
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

describe("Banking Credit Desk template (kv_banking)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_banking");
    expect(template.id).toBe("kv_banking");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("Banking Credit Desk (Tài chính, ngân hàng)");
    expect(template.description.trim()).not.toBe("");
  });

  it("describes the squad with its credit memo keyword and its limits", () => {
    expect(template.description).toContain("tờ trình tín dụng");
    expect(template.description).toContain("gives no investment advice");
  });

  it("has the eight roles of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...KV_BANKING_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = KV_BANKING_ROLE_KEYS.filter((key) => KV_BANKING_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
  });

  it("keeps at most eight members besides the lead", () => {
    const members = template.roles.filter((key) => key !== template.leader);

    expect(members.length).toBeLessThanOrEqual(8);
    expect(members).toHaveLength(7);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of KV_BANKING_ROLE_KEYS) {
      const definition = KV_BANKING_ROLES[key];
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

  it("seats the squad on the Banking floor, from the P2 floor list", () => {
    expect(template.floorName).toBe(KITVIET_P2_FLOOR_NAMES.banking);
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches the pipeline day-chuyen-to-trinh-tin-dung as a core skill", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-to-trinh-tin-dung");
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

    expect(charter).toBe(KV_BANKING_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("tờ trình tín dụng");
  });

  it("draws the decision and advice boundary in the charter: no approval, no investment advice, no return promise, nothing sent", () => {
    const boundaries = template.charter.boundaries;

    expect(boundaries).toContain("Never approves");
    expect(boundaries).toContain("never gives investment advice");
    expect(boundaries).toContain("never promises a return or a profit");
    expect(boundaries).toContain("Nothing leaves the workspace");
  });

  it("gives the lead the generic lead text of the pack and keeps the squad rules in its notes", async () => {
    expect(template.instructions).toBe(await instructionOf("banking_lead"));
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(KV_LEAD_NOTES.kv_banking ?? "").toContain("Squad rules for Banking Credit Desk:");
    expect(KV_LEAD_NOTES.kv_banking ?? "").toContain("Never write that a borrower is approved");
    expect(KV_LEAD_NOTES.kv_banking ?? "").toContain("Investment advice");
    expect(KV_LEAD_NOTES.kv_banking ?? "").toContain("Nothing leaves the workspace");
  });
});

describe("Banking Credit Desk agent instructions", () => {
  it("has an instruction text for every role and no other", async () => {
    const texts = await Promise.all(KV_BANKING_ROLE_KEYS.map((key) => instructionOf(key)));
    expect(texts).toHaveLength(KV_BANKING_ROLE_KEYS.length);
  });

  it("starts each role with its header and its kind text, with one opener", async () => {
    for (const key of KV_BANKING_ROLE_KEYS) {
      const generated = generatedRole(key);
      const text = await instructionOf(key);

      expect(text.startsWith(generated.header), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain(composeKvAgentInstructions({ kind: generated.kind as KvRoleKind }).split("\n\n")[1]);
    }
  });

  it("adds each member's trade body from the vendored agent, and the lead none", async () => {
    for (const key of KV_BANKING_ROLE_KEYS) {
      const body = KITVIET_AGENTS.find((agent) => agent.name === key)?.instructions;
      const text = await instructionOf(key);

      if (key === "banking_lead") {
        expect(body).toBeUndefined();
        expect(text).toBe(template.instructions);
      } else {
        expect(body, key).toBeTruthy();
        expect(text.includes(body!), key).toBe(true);
      }
    }
  });

  it("gives each gate exactly one VERDICT line in its body", async () => {
    for (const key of KV_BANKING_ROLE_KEYS) {
      if (KV_BANKING_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("puts the credit memo and advice verdicts in the form the patched gates state", async () => {
    for (const key of ["banking_credit_gate", "banking_advice_gate"] as KvBankingRoleKey[]) {
      const text = await instructionOf(key);

      expect(text, key).toContain("VERDICT: QUA|CHẶN - <lý do một câu>");
      expect(text, key).not.toMatch(/^## Kết luận: QUA \| CHẶN$/m);
    }
  });

  it("keeps the two gates read-only: no write tool and no network tool", () => {
    for (const key of ["banking_credit_gate", "banking_advice_gate"] as KvBankingRoleKey[]) {
      const tools = KITVIET_AGENTS.find((agent) => agent.name === key)?.tools ?? [];

      expect(tools, key).not.toContain("Write");
      expect(tools, key).not.toContain("Edit");
      expect(tools, key).not.toContain("WebSearch");
      expect(tools, key).not.toContain("WebFetch");
    }
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(KV_BANKING_ROLE_KEYS.map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
