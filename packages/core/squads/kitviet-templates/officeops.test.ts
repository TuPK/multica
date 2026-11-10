// @vitest-environment node
import { describe, expect, it } from "vitest";
import { COMPANY_FLOOR_NAMES } from "../company-template";
import { KV_LEAD_NOTES } from "../kitviet-agent-instructions";
import { loadKitvietRoleInstructions } from "../kitviet-packs";
import { KITVIET_BUILTIN_SKILLS as COMMON_PACK_SKILLS } from "../kitviet-skills-data/common";
import { KITVIET_BUILTIN_SKILLS as OFFICEOPS_PACK_SKILLS } from "../kitviet-skills-data/officeops";
import { KV_OFFICEOPS_ROLE_KEYS, KV_OFFICEOPS_ROLES, KV_OFFICEOPS_TEMPLATE } from "./officeops";

const packSkillNames = new Set(
  [...OFFICEOPS_PACK_SKILLS, ...COMMON_PACK_SKILLS].map((skill) => skill.name),
);
const OUTPUT_KINDS = ["issue", "proposal", "kb_document", "report"];
const CRON_FIELD = /^[\d*/,-]+$/;

/** The nine operations skills of spec 5.10 that Office Admin no longer defers. */
const OPS_SKILLS = [
  "bao-cao-hanh-chinh-thang",
  "dat-phong-hop-hau-can",
  "dat-ve-du-toan-cong-tac",
  "luu-tru-ho-so-van-phong",
  "quan-ly-van-phong-pham",
  "so-tai-san-van-phong",
  "theo-doi-hop-dong-dich-vu-vp",
  "tiep-khach-thu-cam-on",
  "to-chuc-su-kien-hoi-nghi",
];

describe("Office Ops (kv_officeops) squad template", () => {
  const t = KV_OFFICEOPS_TEMPLATE;

  it("has the id, name and floor of the Kit Viet spec", () => {
    expect(t.id).toBe("kv_officeops");
    expect(t.name).toBe("Office Ops (Vận hành văn phòng)");
    expect(t.floorName).toBe("Executive");
  });

  it("has a one-line description", () => {
    expect(t.description.trim()).not.toBe("");
    expect(t.description).not.toContain("\n");
  });

  it("has every role of the squad, each defined once", () => {
    expect(t.roles).toEqual([...KV_OFFICEOPS_ROLE_KEYS]);
    expect(new Set(t.roles).size).toBe(t.roles.length);
    expect(Object.keys(KV_OFFICEOPS_ROLES).sort()).toEqual([...KV_OFFICEOPS_ROLE_KEYS].sort());
  });

  it("has exactly one lead, and it is the leader", () => {
    const leads = KV_OFFICEOPS_ROLE_KEYS.filter((key) => KV_OFFICEOPS_ROLES[key].agentTemplate === "kv_lead");
    expect(leads).toEqual([t.leader]);
    expect(t.roles[0]).toBe(t.leader);
    for (const key of KV_OFFICEOPS_ROLE_KEYS.filter((k) => k !== t.leader)) {
      expect(KV_OFFICEOPS_ROLES[key].agentTemplate, key).not.toBe("kv_lead");
    }
  });

  it("keeps the squad within the desk limit: at most 8 members besides the lead", () => {
    const members = KV_OFFICEOPS_ROLE_KEYS.filter((key) => key !== t.leader);
    expect(members.length).toBeLessThanOrEqual(8);
    expect(t.roles.length).toBeLessThanOrEqual(9);
  });

  it("seats the squad on a default floor", () => {
    expect(Object.values(COMPANY_FLOOR_NAMES)).toContain(t.floorName);
  });

  it("gives each role a name, an English description within the limit, and a roster line", () => {
    const names = new Set<string>();
    for (const key of KV_OFFICEOPS_ROLE_KEYS) {
      const role = KV_OFFICEOPS_ROLES[key];
      expect(role.key, key).toBe(key);
      expect(role.name.trim(), key).not.toBe("");
      expect(names.has(role.name), role.name).toBe(false);
      names.add(role.name);
      expect(role.description.trim(), key).not.toBe("");
      expect([...role.description].length, key).toBeLessThanOrEqual(255);
      expect(role.role.trim(), key).not.toBe("");
    }
  });

  it("keeps the type prefix out of the roster lines", () => {
    for (const key of KV_OFFICEOPS_ROLE_KEYS) {
      expect(KV_OFFICEOPS_ROLES[key].role, key).not.toMatch(/^(Thu thập|Thực thi|Gác cổng)/);
    }
  });

  it("builds each role's instructions from its header, its kind text and its body, with one opener", async () => {
    for (const key of KV_OFFICEOPS_ROLE_KEYS) {
      const text = await loadKitvietRoleInstructions(key);
      if (text === undefined) throw new Error(`no Kit Viet instructions for ${key}`);
      expect(text.startsWith(`You are the ${KV_OFFICEOPS_ROLES[key].name} in an Office Ops`), key).toBe(true);
      expect(text.match(/^You are /gm)?.length ?? 0, key).toBe(1);
      expect(text, key).toContain("unattended");
    }
  });

  it("gives the lead the instructions of the loader, with the Office Ops flow", async () => {
    expect(t.instructions).toBe(await loadKitvietRoleInstructions("officeops_lead"));
    expect(t.instructions).toContain("kv--squad-lead");
    expect(t.instructions).toContain((KV_LEAD_NOTES.kv_officeops ?? "").split("\n")[0]);
  });

  it("writes the lead flow with its stops, its stage order and its no-action rule", () => {
    expect(KV_LEAD_NOTES.kv_officeops ?? "").toContain("Stop 1");
    expect(KV_LEAD_NOTES.kv_officeops ?? "").toContain("Stage order");
    expect(KV_LEAD_NOTES.kv_officeops ?? "").toContain("Nothing is booked");
    expect(KV_LEAD_NOTES.kv_officeops ?? "").toContain("multica proposal create --squad");
  });

  it("has no workflow", () => {
    expect(t.workflow).toBeNull();
  });

  it("names only built-in skills that exist in the pack", () => {
    const names = [...t.builtinSkills, ...(t.leaderSkills ?? [])];
    for (const roleNames of Object.values(t.roleSkills ?? {})) names.push(...(roleNames ?? []));

    expect(new Set(t.builtinSkills).size).toBe(t.builtinSkills.length);
    for (const name of names) {
      expect(packSkillNames.has(name), name).toBe(true);
    }
  });

  it("attaches each of the nine operations skills once, to the core or to a role that runs it", () => {
    const attached = [...t.builtinSkills, ...Object.values(t.roleSkills ?? {}).flatMap((skills) => [...(skills ?? [])])];
    const ops = attached.map((name) => name.replace(/^kv--/, "")).filter((name) => OPS_SKILLS.includes(name));
    expect([...ops].sort()).toEqual([...OPS_SKILLS].sort());
  });

  it("is an authored flow: no upstream pipeline skill is attached", () => {
    const names = [...t.builtinSkills, ...Object.values(t.roleSkills ?? {}).flatMap((skills) => [...(skills ?? [])])];
    for (const name of names) expect(name.startsWith("day-chuyen-"), name).toBe(false);
  });

  it("gives only roles of this squad their extra skills, and each role at most six", () => {
    for (const [key, skills] of Object.entries(t.roleSkills ?? {})) {
      expect(KV_OFFICEOPS_ROLE_KEYS, key).toContain(key);
      expect(skills?.length ?? 0, key).toBeLessThanOrEqual(6);
    }
  });

  it("holds the core skills and the lead skills of the pack", () => {
    expect(t.builtinSkills).toContain("kv--delivery-core");
    expect(t.leaderSkills).toEqual(["kv--squad-lead", "kv--company-profile"]);
  });

  it("has a valid charter", () => {
    const c = t.charter;
    expect(c.mission.trim()).not.toBe("");
    expect(c.responsibilities.length).toBeGreaterThan(0);
    expect(new Set(c.responsibilities.map((r) => r.key)).size).toBe(c.responsibilities.length);
    for (const r of c.responsibilities) expect(r.text.trim(), r.key).not.toBe("");
    expect(c.outputs.length).toBeGreaterThan(0);
    for (const output of c.outputs) {
      expect(OUTPUT_KINDS, output.key).toContain(output.kind);
      expect(output.cadenceHint.trim(), output.key).not.toBe("");
    }
    expect(c.boundaries).toContain("Nothing leaves the workspace");
    expect(c.escalation.trim()).not.toBe("");
    expect(c.intake.trim()).not.toBe("");
  });

  it("has no routines: every request runs on its issue", () => {
    expect(t.routines).toEqual([]);
  });

  it("gives every routine a five-field cron and starts it paused", () => {
    for (const routine of t.routines) {
      const fields = routine.cron.trim().split(/\s+/);
      expect(fields.length, routine.title).toBe(5);
      for (const field of fields) expect(field, routine.title).toMatch(CRON_FIELD);
      expect(routine.startPaused, routine.title).toBe(true);
    }
  });
});
