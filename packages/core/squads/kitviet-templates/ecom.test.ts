// @vitest-environment node
import { describe, expect, it } from "vitest";
import { COMPANY_FLOOR_NAMES } from "../company-template";
import { composeKvAgentInstructions, KV_LEAD_NOTES, KV_ROLE_KINDS, type KvRoleKind } from "../kitviet-agent-instructions";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../kitviet-skills-data/ecom";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../kitviet-skills-data/common";
import { KV_ECOM_ROLE_KEYS, KV_ECOM_ROLES, KV_ECOM_TEMPLATE, KV_ECOM_FLOOR_NAME, type KvEcomRoleKey } from "./ecom";

const KITVIET_SQUAD = kitvietSquad("kv_ecom");

const template = KV_ECOM_TEMPLATE;
const packSkillNames = new Set([...COMMON_SKILLS, ...KITVIET_BUILTIN_SKILLS].map((skill) => skill.name));
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];

/** Floors this squad may sit on: the default floors, plus the one spec 9.1 and Q4 document as new. */
const DOCUMENTED_NEW_FLOORS = ["Revenue"];

/** The generated header and kind of a role, from the names-only squad data. */
function headerOf(role: KvEcomRoleKey): { header: string; kind: string } {
  const found = findKitvietRole(role);
  if (!found) throw new Error(`The ecom pack has no role for ${role}.`);
  return found.role;
}

async function instructionOf(role: KvEcomRoleKey): Promise<string> {
  const text = await loadKitvietRoleInstructions(role);
  if (text === undefined) throw new Error(`no Kit Viet instructions for ${role}`);
  return text;
}

describe("E-commerce Shop Ops template (kv_ecom)", () => {
  it("names the squad and its pack the way the generated data does", () => {
    expect(template.id).toBe("kv_ecom");
    expect(template.name).toBe("E-commerce Shop Ops (Bán hàng online)");
    expect(KITVIET_SQUAD.templateId).toBe(template.id);
    expect(KITVIET_SQUAD.name).toBe(template.name);
    expect(template.description.trim()).not.toBe("");
    expect(template.description.includes("\n")).toBe(false);
  });

  it("has a definition for every role it lists, and no extra definition", () => {
    expect(template.roles).toEqual([...KV_ECOM_ROLE_KEYS]);
    for (const role of KV_ECOM_ROLE_KEYS) expect(KV_ECOM_ROLES[role], role).toBeDefined();
    expect(Object.keys(KV_ECOM_ROLES).sort()).toEqual([...KV_ECOM_ROLE_KEYS].sort());
    for (const [key, role] of Object.entries(KV_ECOM_ROLES)) {
      expect(role.key).toBe(key);
      expect(role.name.trim()).not.toBe("");
      expect(role.description.trim()).not.toBe("");
      expect(role.description.includes("\n")).toBe(false);
    }
  });

  it("has exactly one lead, and it is the first role", () => {
    const leads = KV_ECOM_ROLE_KEYS.filter((role) => KV_ECOM_ROLES[role].agentTemplate === "kv_lead");
    expect(leads).toEqual([template.leader]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("keeps the desk to nine seats: a lead and at most eight members", () => {
    expect(template.roles.length - 1).toBeLessThanOrEqual(8);
    expect(template.roles.length).toBe(9);
  });

  it("gives every role a name no other role of this squad uses", () => {
    const names = KV_ECOM_ROLE_KEYS.map((role) => KV_ECOM_ROLES[role].name);
    expect(new Set(names).size).toBe(names.length);
  });

  it("seats the squad on the documented new floor, Revenue, and only on a valid floor", () => {
    expect(template.floorName).toBe(KV_ECOM_FLOOR_NAME);
    expect(template.floorName).toBe("Revenue");
    expect([...Object.values(COMPANY_FLOOR_NAMES), ...DOCUMENTED_NEW_FLOORS]).toContain(template.floorName);
  });

  it("imports only built-in skills the pack ships, and attaches role skills to roles the squad has", () => {
    const attached = [
      ...template.builtinSkills,
      ...(template.leaderSkills ?? []),
      ...Object.values(template.roleSkills ?? {}).flat(),
    ];
    expect(attached.filter((name) => !packSkillNames.has(name))).toEqual([]);
    for (const role of Object.keys(template.roleSkills ?? {})) {
      expect(template.roles, role).toContain(role);
    }
    expect(template.builtinSkills).toEqual(KITVIET_SQUAD.builtinSkills);
    expect(template.leaderSkills).toEqual(KITVIET_SQUAD.leaderSkills);
    expect(template.roleSkills).toEqual({
      ecom_intake: KITVIET_SQUAD.roleSkills.ecom_intake,
      ecom_rival_scout: KITVIET_SQUAD.roleSkills.ecom_rival_scout,
      ecom_listing_drafter: KITVIET_SQUAD.roleSkills.ecom_listing_drafter,
      ecom_cskh_drafter: KITVIET_SQUAD.roleSkills.ecom_cskh_drafter,
      ecom_policy_gate: KITVIET_SQUAD.roleSkills.ecom_policy_gate,
      ecom_margin_gate: KITVIET_SQUAD.roleSkills.ecom_margin_gate,
    });
  });

  it("keeps no more than six role skills on any role", () => {
    for (const [role, skills] of Object.entries(template.roleSkills ?? {})) {
      expect(skills.length, role).toBeLessThanOrEqual(6);
    }
  });

  it("keeps the core skills the desk needs: the delivery core, the pipeline and the squad lead", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-len-san-pham-moi");
    expect(template.builtinSkills).toContain("kv--delivery-core");
    expect(template.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("leaves the shared Viet giong Gen Z skill to the Marketing Studio pack", () => {
    expect(template.builtinSkills).not.toContain("kv--viet-giong-gen-z");
    expect(Object.values(template.roleSkills ?? {}).flat()).not.toContain("kv--viet-giong-gen-z");
  });

  it("writes a complete charter with valid outputs and intake words for the marketplace work", () => {
    const charter = template.charter;
    expect(charter.mission.trim()).not.toBe("");
    expect(charter.boundaries.trim()).not.toBe("");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake.trim()).not.toBe("");
    expect(charter.intake).toContain("đăng sản phẩm");
    expect(charter.intake).toContain("sàn");
    expect(charter.responsibilities.length).toBeGreaterThan(0);
    expect(new Set(charter.responsibilities.map((item) => item.key)).size).toBe(charter.responsibilities.length);
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) {
      expect(OUTPUT_KINDS, output.key).toContain(output.kind);
      expect(output.cadenceHint.trim(), output.key).not.toBe("");
    }
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
  });

  it("has no routine: listings and reports run on request, not on a schedule", () => {
    expect(template.routines).toEqual([]);
  });

  it("starts the lead from its own notes and states the no-post rule and the fee stop", async () => {
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(template.instructions).toContain(KV_LEAD_NOTES.kv_ecom ?? "");
    expect(template.instructions).toContain("Nothing leaves the workspace");
    expect(template.instructions).toContain("Fees are inputs, not facts");
    expect(template.workflow).toBeNull();
  });

  it("keeps the lead's text equal to the generated lead header with its notes", () => {
    const header = headerOf("ecom_lead");
    expect(template.instructions).toBe(
      composeKvAgentInstructions({ kind: "kv_lead" as KvRoleKind, header: header.header, notes: KV_LEAD_NOTES.kv_ecom }),
    );
  });
});

describe("E-commerce Shop Ops role instructions", () => {
  it("composes each role's text from its header, its kind text and its pack body, in that order", async () => {
    for (const role of KV_ECOM_ROLE_KEYS) {
      if (role === "ecom_lead") continue;
      const header = headerOf(role);
      const text = await instructionOf(role);
      expect(text.startsWith(header.header), role).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, role).toBe(1);
      const body = KITVIET_AGENTS.find((agent) => agent.name === role)?.instructions;
      expect(body, role).toBeDefined();
      expect(text, role).toBe(
        composeKvAgentInstructions({ kind: header.kind as KvRoleKind, header: header.header, body }),
      );
    }
  });

  it("gives each role the kind its agent template expects", () => {
    for (const role of KV_ECOM_ROLE_KEYS) {
      expect(KV_ROLE_KINDS, role).toContain(KV_ECOM_ROLES[role].agentTemplate);
      expect(headerOf(role).kind, role).toBe(KV_ECOM_ROLES[role].agentTemplate);
    }
  });

  it("gives the three gates the VERDICT line and no drafting role gets one", async () => {
    const gates = KV_ECOM_ROLE_KEYS.filter((role) => KV_ECOM_ROLES[role].agentTemplate === "kv_gate");
    expect(gates).toHaveLength(3);
    for (const gate of gates) expect(await instructionOf(gate), gate).toContain("VERDICT:");
    const drafters = KV_ECOM_ROLE_KEYS.filter((role) => KV_ECOM_ROLES[role].agentTemplate === "kv_drafter");
    expect(drafters).toHaveLength(2);
    for (const role of drafters) expect(await instructionOf(role), role).not.toContain("VERDICT:");
  });

  it("keeps the squad's role headers free of platform names", () => {
    for (const role of KV_ECOM_ROLE_KEYS) {
      expect(headerOf(role).header).not.toMatch(/Codex|Claude Desktop/);
    }
  });

  it("drops the dated advertising-law citation from the listing skill and points to the register", () => {
    const skill = KITVIET_BUILTIN_SKILLS.find((item) => item.name === "kv--mo-ta-san-pham-shop");
    expect(skill).toBeDefined();
    expect(skill?.content).not.toContain("Luật Quảng cáo");
    expect(skill?.content).toContain("[chưa có sổ quy định]");
  });
});
