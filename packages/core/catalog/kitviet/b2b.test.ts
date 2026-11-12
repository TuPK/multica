// @vitest-environment node
import { describe, expect, it } from "vitest";
import { COMPANY_FLOOR_NAMES } from "../../squads/company-template";
import { composeKvAgentInstructions, KV_LEAD_NOTES, KV_ROLE_KINDS, type KvRoleKind } from "../../squads/kitviet-agent-instructions";
import { loadKitvietRoleInstructions } from "../../squads/kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../../squads/kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../generated/kitviet/b2b";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../generated/kitviet/common";
import { SQUAD_ROLES, SQUAD_TEMPLATES, type SquadRoleKey } from "../../squads/templates";

// The squad is catalog data (catalog/squads/kv_b2b), assembled into SQUAD_TEMPLATES and SQUAD_ROLES.
const KV_B2B_ROLE_KEYS = SQUAD_TEMPLATES.find((t) => t.id === "kv_b2b")!.roles;
const KV_B2B_ROLES = Object.fromEntries(SQUAD_TEMPLATES.find((t) => t.id === "kv_b2b")!.roles.map((key) => [key, SQUAD_ROLES[key]])) as typeof SQUAD_ROLES;
const KV_B2B_TEMPLATE = SQUAD_TEMPLATES.find((t) => t.id === "kv_b2b")!;
type KvB2bRoleKey = SquadRoleKey;

const KITVIET_SQUAD = kitvietSquad("kv_b2b");

const template = KV_B2B_TEMPLATE;
const packSkillNames = new Set([...COMMON_SKILLS, ...KITVIET_BUILTIN_SKILLS].map((skill) => skill.name));
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];

/** The generated header and kind of a role, from the names-only squad data. */
function headerOf(role: KvB2bRoleKey): { header: string; kind: string } {
  const found = findKitvietRole(role);
  if (!found) throw new Error(`The b2b pack has no role for ${role}.`);
  return found.role;
}

async function instructionOf(role: KvB2bRoleKey): Promise<string> {
  const text = await loadKitvietRoleInstructions(role);
  if (text === undefined) throw new Error(`no Kit Viet instructions for ${role}`);
  return text;
}

describe("B2B Proposal Desk template (kv_b2b)", () => {
  it("names the squad and its pack the way the generated data does", () => {
    expect(template.id).toBe("kv_b2b");
    expect(template.name).toBe("B2B Proposal Desk (Chào giá B2B)");
    expect(KITVIET_SQUAD.templateId).toBe(template.id);
    expect(KITVIET_SQUAD.name).toBe(template.name);
    expect(template.description.trim()).not.toBe("");
    expect(template.description.includes("\n")).toBe(false);
  });

  it("has a definition for every role it lists, and no extra definition", () => {
    expect(template.roles).toEqual([...KV_B2B_ROLE_KEYS]);
    for (const role of KV_B2B_ROLE_KEYS) expect(KV_B2B_ROLES[role], role).toBeDefined();
    expect(Object.keys(KV_B2B_ROLES).sort()).toEqual([...KV_B2B_ROLE_KEYS].sort());
    for (const [key, role] of Object.entries(KV_B2B_ROLES)) {
      expect(role.key).toBe(key);
      expect(role.name.trim()).not.toBe("");
      expect(role.description.trim()).not.toBe("");
      expect(role.description.includes("\n")).toBe(false);
    }
  });

  it("has exactly one lead, and it is the first role", () => {
    const leads = KV_B2B_ROLE_KEYS.filter((role) => KV_B2B_ROLES[role].agentTemplate === "kv_lead");
    expect(leads).toEqual([template.leader]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("keeps the desk to nine seats: a lead and at most eight members", () => {
    expect(template.roles.length - 1).toBeLessThanOrEqual(8);
    expect(template.roles.length).toBe(9);
  });

  it("gives every role a name no other role of this squad uses", () => {
    const names = KV_B2B_ROLE_KEYS.map((role) => KV_B2B_ROLES[role].name);
    expect(new Set(names).size).toBe(names.length);
  });

  it("seats the squad on a default floor, Launch", () => {
    expect(Object.values(COMPANY_FLOOR_NAMES)).toContain(template.floorName);
    expect(template.floorName).toBe("Launch");
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
      b2b_quote_drafter: KITVIET_SQUAD.roleSkills.b2b_quote_drafter,
      b2b_company_lookup: KITVIET_SQUAD.roleSkills.b2b_company_lookup,
    });
  });

  it("keeps the core skills the desk needs: the delivery core and the shared authoring skills", () => {
    expect(template.builtinSkills).toContain("kv--delivery-core");
    expect(template.builtinSkills).toContain("kv--day-chuyen-chao-gia-b2b");
    expect(template.builtinSkills).toContain("kv--bien-ban-gap-khach-b2b");
    expect(template.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("writes a complete charter with valid outputs", () => {
    const charter = template.charter;
    expect(charter.mission.trim()).not.toBe("");
    expect(charter.boundaries.trim()).not.toBe("");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake.trim()).not.toBe("");
    expect(charter.responsibilities.length).toBeGreaterThan(0);
    expect(new Set(charter.responsibilities.map((item) => item.key)).size).toBe(charter.responsibilities.length);
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) {
      expect(OUTPUT_KINDS, output.key).toContain(output.kind);
      expect(output.cadenceHint.trim(), output.key).not.toBe("");
    }
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
  });

  it("has no routine, so nothing opens an issue on a schedule", () => {
    expect(template.routines).toEqual([]);
    for (const routine of template.routines) {
      expect(routine.cron.trim().split(/\s+/)).toHaveLength(5);
      expect(routine.issueTitleTemplate).toContain("{{date}}");
    }
  });

  it("starts the lead from the kv_lead text and states the no-send rule", async () => {
    expect(template.instructions).toBe(await instructionOf("b2b_lead"));
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(template.instructions).toContain(KV_LEAD_NOTES.kv_b2b ?? "");
    expect(template.instructions).toContain("Nothing leaves the workspace");
    expect(template.workflow).toBeNull();
  });
});

describe("B2B Proposal Desk role instructions", () => {
  it("composes each role's text from its header, its kind text and its pack body, in that order", async () => {
    for (const role of KV_B2B_ROLE_KEYS) {
      const header = headerOf(role);
      const text = await instructionOf(role);
      expect(text.startsWith(header.header), role).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, role).toBe(1);
      const body = KITVIET_AGENTS.find((agent) => agent.name === role)?.instructions;
      expect(text, role).toBe(
        composeKvAgentInstructions({
          kind: header.kind as KvRoleKind,
          header: header.header,
          body,
          notes: role === "b2b_lead" ? KV_LEAD_NOTES.kv_b2b : undefined,
        }),
      );
    }
  });

  it("gives each role the kind its agent template expects", () => {
    for (const role of KV_B2B_ROLE_KEYS) {
      expect(KV_ROLE_KINDS, role).toContain(KV_B2B_ROLES[role].agentTemplate);
      expect(headerOf(role).kind, role).toBe(KV_B2B_ROLES[role].agentTemplate);
    }
  });

  it("gives the four gates the VERDICT line and no drafting role gets one", async () => {
    const gates = KV_B2B_ROLE_KEYS.filter((role) => KV_B2B_ROLES[role].agentTemplate === "kv_gate");
    expect(gates).toHaveLength(4);
    for (const gate of gates) expect(await instructionOf(gate), gate).toContain("VERDICT:");
    const drafter = KV_B2B_ROLE_KEYS.filter((role) => KV_B2B_ROLES[role].agentTemplate === "kv_drafter");
    for (const role of drafter) expect(await instructionOf(role), role).not.toContain("VERDICT:");
  });

  it("keeps the squad's role headers free of platform names", () => {
    for (const role of KV_B2B_ROLE_KEYS) {
      expect(headerOf(role).header).not.toMatch(/Codex|Claude Desktop/);
    }
  });
});
