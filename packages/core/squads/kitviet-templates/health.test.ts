// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../../agents/instruction-templates";
import { KV_LEAD_NOTES, composeKvAgentInstructions, type KvRoleKind } from "../kitviet-agent-instructions";
import { KITVIET_P2_FLOOR_NAME_LIST, KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { findKitvietRole, kitvietSquad } from "../kitviet-squad-roles";
import { KITVIET_AGENTS, KITVIET_BUILTIN_SKILLS } from "../kitviet-skills-data/health";
import { KITVIET_BUILTIN_SKILLS as COMMON_SKILLS } from "../kitviet-skills-data/common";
import {
  HEALTH_CHARTER,
  HEALTH_ROLE_KEYS,
  HEALTH_ROLES,
  type HealthRoleKey,
  KV_HEALTH_TEMPLATE as template,
} from "./health";

const KITVIET_SQUAD = kitvietSquad("kv_health");
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

describe("Healthcare template (kv_health)", () => {
  it("is the squad the generated pack describes", () => {
    expect(KITVIET_SQUAD.templateId).toBe("kv_health");
    expect(template.id).toBe("kv_health");
    expect(template.name).toBe(KITVIET_SQUAD.name);
    expect(template.name).toBe("Healthcare (Y tế)");
    expect(template.description.trim()).not.toBe("");
    expect(template.description).not.toContain("\n");
  });

  it("states that it writes administrative and educational drafts only", () => {
    expect(template.description).toContain("administrative and educational drafts only");
    expect(template.description).toContain("no diagnosis, prescription or treatment advice");
  });

  it("has the nine roles of the generated roster, led by the one lead", () => {
    expect(template.roles).toEqual(KITVIET_SQUAD.roles.map((role) => role.key));
    expect([...template.roles]).toEqual([...HEALTH_ROLE_KEYS]);
    expect(template.roles[0]).toBe(template.leader);
    expect(new Set(template.roles).size).toBe(template.roles.length);
  });

  it("names exactly one lead, built from the lead instruction template", () => {
    const leads = HEALTH_ROLE_KEYS.filter((key) => HEALTH_ROLES[key].agentTemplate === "kv_lead");

    expect(leads).toEqual([template.leader]);
    expect(HEALTH_ROLES[template.leader as HealthRoleKey].agentTemplate).toBe("kv_lead");
  });

  it("keeps at most eight members besides the lead", () => {
    const members = template.roles.filter((key) => key !== template.leader);

    expect(members.length).toBeLessThanOrEqual(8);
    expect(members).toHaveLength(8);
  });

  it("gives every role a definition whose key, name and description are complete", () => {
    const names = new Set<string>();

    for (const key of HEALTH_ROLE_KEYS) {
      const definition = HEALTH_ROLES[key];
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

  it("seats the squad on the Healthcare floor, which the P2 floor list names", () => {
    expect(template.floorName).toBe("Healthcare");
    expect(template.floorName).toBe(KITVIET_P2_FLOOR_NAMES.health);
    expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(template.floorName);
  });

  it("has no routine and no workflow (the squad runs on requests only)", () => {
    expect(template.routines).toEqual([]);
    expect(template.workflow).toBeNull();
  });

  it("attaches the pipeline day-chuyen-tai-lieu-y-te as a core skill", () => {
    expect(template.builtinSkills).toContain("kv--day-chuyen-tai-lieu-y-te");
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

  it("writes a charter that states the scope, the patient-data rule and the nothing-leaves rule", () => {
    const charter = template.charter;
    const keys = charter.responsibilities.map((item) => item.key);

    expect(charter).toBe(HEALTH_CHARTER);
    expect(charter.mission.trim()).not.toBe("");
    expect(new Set(keys).size).toBe(keys.length);
    expect(keys.length).toBeGreaterThan(0);
    for (const item of charter.responsibilities) expect(item.text.trim(), item.key).not.toBe("");
    expect(charter.outputs.length).toBeGreaterThan(0);
    for (const output of charter.outputs) expect(OUTPUT_KINDS, output.key).toContain(output.kind);

    expect(charter.boundaries).toContain("administrative and educational drafts only");
    expect(charter.boundaries).toContain("Never gives a diagnosis, prescribes or changes a medicine or dose");
    expect(charter.boundaries).toContain("advises on treatment");
    expect(charter.boundaries).toContain("Patient data");
    expect(charter.boundaries).toContain("stays in the workspace");
    expect(charter.boundaries).toContain("Nothing leaves the workspace");
    expect(charter.escalation.trim()).not.toBe("");
    expect(charter.intake).toContain("SOP");
    expect(charter.intake).toContain("phác đồ");
    expect(charter.intake).toContain("hạn dùng thuốc");
  });

  it("makes the lead keep the scope gate, the clinician stop and the nothing-sent rule", async () => {
    expect(template.instructions).toContain("Your job: turn a request into sub-issues");
    expect(template.instructions).toContain(KV_LEAD_NOTES.kv_health ?? "");
    expect(template.instructions).toContain("Scope boundary");
    expect(template.instructions).toContain("CHẶN");
    expect(template.instructions).toContain("Nothing leaves the workspace");
    expect(template.instructions).toBe(
      composeKvAgentInstructions({
        kind: "kv_lead" as KvRoleKind,
        header: generatedRole("health_lead").header,
        notes: KV_LEAD_NOTES.kv_health ?? "",
      }),
    );
  });
});

describe("Healthcare agent instructions", () => {
  it("has an instruction text for every member role", async () => {
    const members = HEALTH_ROLE_KEYS.filter((key) => key !== template.leader);
    const texts = await Promise.all(members.map((key) => instructionOf(key)));
    expect(texts).toHaveLength(members.length);
  });

  it("starts each member role with its header and its kind text, with one opener", async () => {
    for (const key of HEALTH_ROLE_KEYS.filter((k) => k !== template.leader)) {
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
    for (const key of HEALTH_ROLE_KEYS) {
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
    for (const key of HEALTH_ROLE_KEYS) {
      if (HEALTH_ROLES[key].agentTemplate !== "kv_gate") continue;
      const verdicts = (await instructionOf(key)).match(/^VERDICT:/gm) ?? [];
      expect(verdicts, key).toHaveLength(1);
    }
  });

  it("gives each gate the severity vocabulary of the pack, with no old word", async () => {
    for (const key of HEALTH_ROLE_KEYS) {
      if (HEALTH_ROLES[key].agentTemplate !== "kv_gate") continue;
      const text = await instructionOf(key);
      expect(text, key).toContain("Chặn");
      expect(text, key).toContain("Nên sửa");
      expect(text, key).toContain("Góp ý");
      for (const word of ["Lưu ý", "Nặng", "Vừa", "Nhẹ", "Phải sửa"]) expect(text.includes(word), `${key}: ${word}`).toBe(false);
    }
  });

  it("leaves no reference to the upstream run folder, the file-based protocol, or Claude's own files", async () => {
    const texts = [
      template.instructions,
      template.charter.boundaries,
      template.charter.escalation,
      ...(await Promise.all(HEALTH_ROLE_KEYS.filter((k) => k !== template.leader).map((key) => instructionOf(key)))),
    ];

    for (const text of texts) {
      expect(text).not.toMatch(/\.kv-run|ket-qua\/|CLAUDE\.md|AGENTS\.md|phiên chính/);
      expect(text).not.toMatch(/\bkv:[a-z]/);
      expect(text).not.toMatch(/\d\d-[a-z-]+\.md/);
    }
  });
});
