// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { PackPreview } from "@multica/core/types";
import {
  buildCommitRequest,
  extraAgentKey,
  initialChoices,
  installBlocker,
  oneAgentPerSkill,
  type PackChoices,
} from "./pack-choices";

const base: PackPreview = {
  digest: "sha256:" + "a".repeat(64),
  format: "claude_agents",
  id: "",
  name: "Writing pack",
  version: "",
  agents: [
    { key: "writer", name: "Writer", description: "", instructions_bytes: 10, skills: ["style"], skills_declared: true, existing_agent_id: null },
    { key: "editor", name: "Editor", description: "", instructions_bytes: 10, skills: [], skills_declared: false, existing_agent_id: "ag-9" },
  ],
  skills: [
    { name: "style", files: 1, existing_skill_id: null, same_content: false },
    { name: "tone", files: 1, existing_skill_id: "sk-1", same_content: true },
  ],
  leader: null,
  flow: { present: false, validation: null, issues: [] },
  references: [],
  references_omitted: 0,
  ignored: [],
  scan: [],
  issues: [],
};
const skillsOnly: PackPreview = { ...base, format: "skills_only", agents: [] };

function choose(preview: PackPreview, patch: Partial<PackChoices> = {}): PackChoices {
  return { ...initialChoices(preview, "rt-1"), ...patch };
}

describe("initialChoices", () => {
  it("creates the agents that do not exist, uses the ones that do, and starts with no leader when the pack names none", () => {
    const c = initialChoices(base, "rt-1");
    expect(c.agents.writer).toMatchObject({ action: "create", runtimeId: "rt-1" });
    expect(c.agents.editor).toMatchObject({ action: "use_existing", existingAgentId: "ag-9" });
    expect(c.leader).toEqual({ kind: "none" });
    expect(c.squadName).toBe("Writing pack");
  });

  it("takes the leader the pack names, and keeps the existing skill whose content is the same", () => {
    const c = initialChoices({ ...base, leader: "writer" }, "rt-1");
    expect(c.leader).toEqual({ kind: "agent", key: "writer" });
    expect(c.skills).toEqual({ tone: "keep_existing" });
  });

  it("starts a skills-only pack with no agents", () => {
    expect(initialChoices(skillsOnly, "rt-1").extras).toEqual([]);
  });
});

describe("one agent per skill", () => {
  it("makes an agent for each skill with a valid key, named after the skill, with that skill", () => {
    const extras = oneAgentPerSkill(skillsOnly, "rt-1", []);
    expect(extras).toEqual([
      { key: "style", name: "style", runtimeId: "rt-1", skills: ["style"] },
      { key: "tone", name: "tone", runtimeId: "rt-1", skills: ["tone"] },
    ]);
  });

  it("makes keys the server accepts: a-z, 0-9 and _, 2 to 32 characters, starting with a letter, and unique", () => {
    const pack = { ...skillsOnly, skills: ["Brand Voice!", "9 lives", "x", "Brand voice", "a".repeat(60)].map((name) => ({ name, files: 1, existing_skill_id: null, same_content: false })) };
    const keys = oneAgentPerSkill(pack, "rt-1", []).map((e) => e.key);
    for (const key of keys) expect(key).toMatch(/^[a-z][a-z0-9_]{1,31}$/);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it("does not repeat a key an agent of the squad has", () => {
    expect(extraAgentKey("Style", new Set(["style"]))).toBe("style_2");
  });
});

describe("installBlocker", () => {
  it("says a pack with agents and no leader needs one", () => {
    expect(installBlocker(base, choose(base))).toBe("leader");
    expect(installBlocker(base, choose(base, { leader: { kind: "agent", key: "writer" } }))).toBeNull();
    expect(installBlocker(base, choose(base, { leader: { kind: "generic" }, genericRuntimeId: "rt-1" }))).toBeNull();
  });

  it("does not count a leader that is skipped", () => {
    const skipped = choose(base, { leader: { kind: "agent", key: "writer" } });
    skipped.agents.writer = { ...skipped.agents.writer!, action: "skip" };
    expect(installBlocker(base, skipped)).toBe("leader");
  });

  it("lets a skills-only pack with no agents install its skills", () => {
    expect(installBlocker(skillsOnly, choose(skillsOnly))).toBeNull();
    const withExtra = choose(skillsOnly, { extras: oneAgentPerSkill(skillsOnly, "rt-1", []) });
    expect(installBlocker(skillsOnly, withExtra)).toBe("leader");
    expect(installBlocker(skillsOnly, { ...withExtra, leader: { kind: "agent", key: "style" } })).toBeNull();
  });

  it("blocks on a scan that blocks, and on errors of the pack, before anything else", () => {
    const blocked = { ...base, scan: [{ path: "a", severity: "block" as const, rule: "r" }] };
    expect(installBlocker(blocked, choose(blocked))).toBe("blocked");
    const errors = { ...base, issues: [{ code: "x", severity: "error" as const, message: "m" }] };
    expect(installBlocker(errors, choose(errors))).toBe("errors");
  });

  it("needs the tick for a warning of the scan", () => {
    const warned = { ...base, scan: [{ path: "a", severity: "warn" as const, rule: "r" }] };
    const c = choose(warned, { leader: { kind: "agent", key: "writer" } });
    expect(installBlocker(warned, c)).toBe("ack");
    expect(installBlocker(warned, { ...c, ackScan: true })).toBeNull();
    // A note does not need one.
    const noted = { ...base, scan: [{ path: "a", severity: "info" as const, rule: "r" }] };
    expect(installBlocker(noted, choose(noted, { leader: { kind: "agent", key: "writer" } }))).toBeNull();
  });

  it("needs a runtime for every agent that is created, and a name for the squad", () => {
    const c = choose(base, { leader: { kind: "agent", key: "writer" } });
    c.agents.writer = { ...c.agents.writer!, runtimeId: "" };
    expect(installBlocker(base, c)).toBe("runtime");
    const unnamed = choose(base, { leader: { kind: "agent", key: "writer" }, squadName: "  " });
    expect(installBlocker(base, unnamed)).toBe("name");
  });
});

describe("buildCommitRequest", () => {
  it("sends the choices the server reads, and only what applies", () => {
    const choices = choose({ ...base, leader: "writer" });
    choices.agents.writer = { ...choices.agents.writer!, name: "Lead writer" };
    choices.skills = { tone: "import_as_new" };
    const request = buildCommitRequest({ ...base, leader: "writer" }, choices, false);
    expect(request).toEqual({
      digest: base.digest,
      squad: { name: "Writing pack" },
      agents: [
        { key: "writer", action: "create", runtime_id: "rt-1", name: "Lead writer" },
        { key: "editor", action: "use_existing", existing_agent_id: "ag-9" },
      ],
      leader: { agent: "writer" },
      skills: { tone: "import_as_new" },
      ack_scan_warnings: false,
    });
  });

  it("asks for a generic leader with its runtime, and for the extra agents the person added", () => {
    const choices = choose(skillsOnly, { extras: oneAgentPerSkill(skillsOnly, "rt-1", []), leader: { kind: "generic" }, genericRuntimeId: "rt-2" });
    const request = buildCommitRequest(skillsOnly, choices, false);
    expect(request.leader).toEqual({ generic: true, runtime_id: "rt-2" });
    expect(request.extra_agents).toEqual([
      { key: "style", name: "style", instructions: "", runtime_id: "rt-1", skills: ["style"] },
      { key: "tone", name: "tone", instructions: "", runtime_id: "rt-1", skills: ["tone"] },
    ]);
    expect(request.agents).toEqual([]);
  });

  it("asks for a draft of the flow only when the pack has one and a squad is made, and marks a new install", () => {
    const withFlow = { ...base, leader: "writer", flow: { present: true, validation: null, issues: [] } };
    const choices = choose(withFlow, { draftFromFlow: true });
    expect(buildCommitRequest(withFlow, choices, true)).toMatchObject({ create_draft_from_flow: true, new_install: true });
    expect(buildCommitRequest(base, choose(base, { draftFromFlow: true }), false)).not.toHaveProperty("create_draft_from_flow");
  });

  it("sends no squad when the install makes none", () => {
    const request = buildCommitRequest(skillsOnly, choose(skillsOnly), false);
    expect(request).not.toHaveProperty("squad");
    expect(request).not.toHaveProperty("leader");
  });
});
