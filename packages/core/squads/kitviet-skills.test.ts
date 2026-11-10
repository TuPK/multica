// @vitest-environment node
// Guards the generated Kit Viet pack: no leftovers from the upstream (Pro ads, platform notes, the
// shared run folder), references that resolve inside each squad, one VERDICT line per gate, every
// patch applied, names that match their folders, size ceilings, and freshness against the generator.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import { KV_ROLE_KINDS } from "./kitviet-agent-instructions";
import * as adminPack from "./kitviet-skills-data/admin";
import * as b2bPack from "./kitviet-skills-data/b2b";
import * as accountingPack from "./kitviet-skills-data/accounting";
import * as commonPack from "./kitviet-skills-data/common";
import * as legalPack from "./kitviet-skills-data/legal";
import * as marketingPack from "./kitviet-skills-data/marketing";
import { KITVIET_SQUADS } from "./kitviet-skills-data/squads";
import type { KitvietAgent, KitvietSkill, KitvietSquad } from "./kitviet-skills-data/types";

const here = dirname(fileURLToPath(import.meta.url));
const upstreamDir = join(here, "kitviet-upstream");
const patchesDir = join(here, "kitviet-patches");
const authoredDir = join(here, "kitviet-skills");
const squadsConfig = JSON.parse(readFileSync(join(here, "kitviet-squads.json"), "utf8")) as {
  sharedBuiltinSkills: string[];
  squads: Record<
    string,
    { trade: string; builtinSkills: string[]; roleSkills?: Record<string, string[]>; deferredSkills?: string[] }
  >;
};
const rewrites = JSON.parse(readFileSync(join(here, "kitviet-rewrites.json"), "utf8")) as {
  droppedSkills: string[];
  renamedSkills: Record<string, Record<string, string>>;
};

const PACKS = [
  { id: "common", module: commonPack },
  { id: "admin", module: adminPack },
  { id: "b2b", module: b2bPack },
  { id: "marketing", module: marketingPack },
  { id: "legal", module: legalPack },
  { id: "accounting", module: accountingPack },
] as const;

const SQUAD_PACKS = PACKS.filter((pack) => pack.id !== "common");

// Every skill in every pack, and the squad config of each squad pack.
const allSkills: KitvietSkill[] = PACKS.flatMap((pack) => [...pack.module.KITVIET_BUILTIN_SKILLS]);
const skillByName = new Map(allSkills.map((skill) => [skill.name, skill]));
const squadConfigs = new Map<string, KitvietSquad>(KITVIET_SQUADS.map((squad) => [squad.pack, squad]));

// The text a squad's body is made of: its attached skills (core and role skills) and its agents.
function squadSkills(squad: KitvietSquad): KitvietSkill[] {
  const attached = new Set([...squad.builtinSkills, ...squad.leaderSkills, ...Object.values(squad.roleSkills).flat()]);
  return allSkills.filter((skill) => attached.has(skill.name));
}

function allTextOf(skill: KitvietSkill): string {
  return [skill.description, skill.content, ...skill.files.map((file) => file.content)].join("\n");
}

function allAgents(): KitvietAgent[] {
  return SQUAD_PACKS.flatMap((pack) => [...pack.module.KITVIET_AGENTS]);
}

const FORBIDDEN: [RegExp, string][] = [
  [/kv:pro(?![\w-])/, "Pro block reference"],
  [/mochistudio/i, "vendor advertising"],
  [/Bản Pro/, "Pro advertising"],
  [/\.kv-run/, "shared run folder"],
  [/CLAUDE\.md/, "CLAUDE.md profile"],
  [/AGENTS\.md/, "AGENTS.md profile"],
  [/Claude Desktop/, "platform name"],
  [/Codex/, "platform name"],
  [/ket-qua\//, "result folder"],
  [/phiên chính/i, "main session"],
  [/\d\d-[a-z-]+\.md/, "numbered stage file"],
  [/kv:[a-z0-9-]/, "unresolved kv: reference"],
  [/(?<![\w-])kv-(delivery-core|squad-lead|company-profile)/, "authored folder name instead of its bundle name"],
];

describe("Kit Viet pack leftovers", () => {
  it("has no forbidden text in any generated skill, reference file or agent body", () => {
    const hits: string[] = [];
    for (const skill of allSkills) {
      for (const [re, what] of FORBIDDEN) {
        if (re.test(allTextOf(skill))) hits.push(`${skill.name}: ${what}`);
      }
    }
    for (const agent of allAgents()) {
      for (const [re, what] of FORBIDDEN) {
        if (re.test(agent.instructions) || re.test(agent.description)) hits.push(`agent ${agent.name}: ${what}`);
      }
    }
    expect(hits).toEqual([]);
  });

  it("has no platform-specific header in any squad role header", () => {
    for (const squad of KITVIET_SQUADS) {
      for (const role of squad.roles) {
        for (const [re, what] of FORBIDDEN) expect(re.test(role.header), `${role.name}: ${what}`).toBe(false);
      }
    }
  });
});

describe("references resolve inside each squad (F17)", () => {
  for (const pack of SQUAD_PACKS) {
    it(`${pack.id}: every kv-- name in the squad's skills and agents is in its core, leader or role skills`, () => {
      const squad = squadConfigs.get(pack.id);
      if (!squad) throw new Error(`no squad config for ${pack.id}`);
      // The lead holds the leader skills; its kind text names them, so they resolve too.
      const resolution = new Set([...squad.builtinSkills, ...squad.leaderSkills, ...Object.values(squad.roleSkills).flat()]);
      const texts: string[] = [
        ...squadSkills(squad).map(allTextOf),
        ...pack.module.KITVIET_AGENTS.map((agent) => `${agent.description}\n${agent.instructions}`),
      ];
      const dangling = new Set<string>();
      for (const text of texts) {
        for (const match of text.matchAll(/kv--[a-z0-9-]+/g)) {
          if (!resolution.has(match[0])) dangling.add(match[0]);
        }
      }
      expect([...dangling]).toEqual([]);
    });

    it(`${pack.id}: every attached skill exists in the pack set`, () => {
      const squad = squadConfigs.get(pack.id)!;
      const attached = [...squad.builtinSkills, ...squad.leaderSkills, ...Object.values(squad.roleSkills).flat()];
      const missing = attached.filter((name) => !skillByName.has(name));
      expect(missing).toEqual([]);
    });
  }

  it("uses the squad role key, not an upstream agent name, in every squad agent body", () => {
    const upstreamNames = [
      "lap-ke-hoach-dieu-phoi",
      "soat-so-lieu",
      "kiem-dinh-ban-giao",
      "soat-so-lieu-thi-truong",
      "do-tham-doi-thu-web",
    ];
    for (const agent of allAgents()) {
      for (const name of upstreamNames) {
        expect(new RegExp(`(?<![\\w-])${name}(?![\\w-])`).test(agent.instructions), `${agent.name} still names ${name}`).toBe(false);
      }
    }
  });
});

describe("no orphan skills (spec 5.10, K8)", () => {
  // Every skill a squad pack ships is either attached to the squad or to one of its roles, or it is
  // named in the squad's deferredSkills (kitviet-squads.json): the P0 list of trade skills that no
  // squad attaches yet, for import by name. A skill in neither is a leftover.
  for (const pack of SQUAD_PACKS) {
    it(`${pack.id}: every skill is attached, or deferred by name`, () => {
      const config = squadsConfig.squads[pack.id];
      if (!config) throw new Error(`no squad config for ${pack.id}`);
      const squad = squadConfigs.get(pack.id)!;
      const attached = new Set([...squad.builtinSkills, ...squad.leaderSkills, ...Object.values(squad.roleSkills).flat()]);
      const renames = rewrites.renamedSkills[config.trade] ?? {};
      const deferred = new Set((config.deferredSkills ?? []).map((folder) => `kv--${renames[folder] ?? folder}`));
      const trade = pack.module.KITVIET_BUILTIN_SKILLS.map((skill) => skill.name);
      const orphans = trade.filter((name) => !attached.has(name) && !deferred.has(name));
      expect(orphans).toEqual([]);
      const stale = [...deferred].filter((name) => attached.has(name));
      expect(stale).toEqual([]);
    });
  }
});

describe("gates and handovers carry one VERDICT line (F18)", () => {
  const gated = allAgents().filter((agent) => agent.kind === "kv_gate" || agent.kind === "kv_handover");

  it("has gates and handovers to check", () => {
    expect(gated.length).toBeGreaterThan(0);
  });

  for (const agent of gated) {
    it(`${agent.name} has exactly one VERDICT line`, () => {
      const verdicts = agent.instructions.split("\n").filter((line) => line.startsWith("VERDICT:"));
      expect(verdicts).toHaveLength(1);
    });

    it(`${agent.name} has no other verdict vocabulary`, () => {
      expect(/(^|\n)[^\n]*\bĐạt \/|Gửi được \//.test(agent.instructions)).toBe(false);
    });
  }

  it("uses only the known role kinds", () => {
    for (const agent of allAgents()) {
      expect(KV_ROLE_KINDS).toContain(agent.kind);
    }
  });
});

describe("patches apply exactly once", () => {
  const patchFiles = readdirSync(patchesDir).filter((name) => name.endsWith(".json") && name !== "_all.json");

  it("has patch files", () => {
    expect(patchFiles.length).toBeGreaterThan(0);
  });

  for (const file of patchFiles) {
    it(`${file}: every replacement is in the generated text exactly once`, () => {
      const name = file.replace(/\.json$/, "");
      const patches = JSON.parse(readFileSync(join(patchesDir, file), "utf8")) as { find: string; replace: string }[];
      const skill = allSkills.find((candidate) => candidate.name === `kv--${name}` || candidate.name === name);
      const agent = allAgents().find((candidate) => candidate.upstream.includes(name));
      const text = skill ? allTextOf(skill) : agent ? agent.instructions : null;
      expect(text, `no generated skill or agent for ${name}`).not.toBeNull();
      for (const { replace } of patches) {
        const count = text!.split(replace).length - 1;
        expect(count, `${name}: replacement ${replace.slice(0, 60)}`).toBe(1);
      }
    });
  }
});

describe("names and folders", () => {
  it("names every upstream skill after its folder, or its rename for that trade", () => {
    const trades = ["common", ...new Set(Object.values(squadsConfig.squads).map((squad) => squad.trade))];
    for (const trade of trades) {
      const dir = join(upstreamDir, trade, "skills");
      const folders = readdirSync(dir, { withFileTypes: true })
        .filter((entry) => entry.isDirectory() && !rewrites.droppedSkills.includes(entry.name))
        .map((entry) => entry.name);
      for (const folder of folders) {
        const short = rewrites.renamedSkills[trade]?.[folder] ?? folder;
        expect(skillByName.has(`kv--${short}`), `${trade}/${folder} has no skill kv--${short}`).toBe(true);
      }
    }
  });

  it("names every authored skill after its folder, without the kv- prefix", () => {
    const folders = readdirSync(authoredDir, { withFileTypes: true }).filter((entry) => entry.isDirectory());
    expect(folders.length).toBeGreaterThan(0);
    for (const folder of folders) {
      const name = `kv--${folder.name.replace(/^kv-/, "")}`;
      expect(skillByName.has(name), `${folder.name} is not a bundle`).toBe(true);
      const skill = skillByName.get(name)!;
      expect(skill.content).toContain(`name: ${name}\n`);
    }
  });

  it("writes the same name in each skill's frontmatter as in its bundle name", () => {
    for (const skill of allSkills) {
      expect(skill.content.startsWith(`---\nname: ${skill.name}\n`), skill.name).toBe(true);
    }
  });

  it("uses names with no colon, so the folder on disk is safe", () => {
    for (const skill of allSkills) expect(skill.name).toMatch(/^kv--[a-z0-9-]+$/);
  });

  it("collides with no SDLC built-in skill name", () => {
    const sdlc = new Set(SDLC_BUILTIN_SKILLS.map((skill) => skill.name));
    for (const skill of allSkills) expect(sdlc.has(skill.name), skill.name).toBe(false);
  });

  it("has unique names across the Kit Viet packs", () => {
    const names = allSkills.map((skill) => skill.name);
    expect(new Set(names).size).toBe(names.length);
  });
});

describe("shipped files", () => {
  it("ships the upstream LICENSE, NOTICE.md and UPSTREAM.json", () => {
    for (const file of ["LICENSE", "NOTICE.md", "UPSTREAM.json"]) {
      expect(existsSync(join(upstreamDir, file)), file).toBe(true);
    }
  });

  it("gives every skill the upstream source config", () => {
    for (const skill of allSkills) {
      expect(skill.config.source).toBe("kitviet");
      expect(skill.config.license.length).toBeGreaterThan(0);
    }
  });
});

describe("size and description ceilings", () => {
  it("keeps every description under 400 characters", () => {
    for (const skill of allSkills) expect(skill.description.length, skill.name).toBeLessThan(400);
    for (const agent of allAgents()) expect(agent.description.length, agent.name).toBeLessThan(400);
  });

  // The ceiling counts the text an agent reads into its context: SKILL.md and the markdown reference
  // files. Scripts (the export scripts of xuat-*) are copied to the workspace but not read as prose.
  // Accounting is the largest trade by design: its three promoted skills (journal entries, deductible
  // expenses, payroll tax) are the pipeline itself and are read on demand, so it gets a 160 KB ceiling.
  const coreCeilingKb = (id: string) => (id === "accounting" ? 160 : 140);
  for (const pack of SQUAD_PACKS) {
    it(`${pack.id}: the core stays under ${coreCeilingKb(pack.id)} KB of prose and 30 skills`, () => {
      const squad = squadConfigs.get(pack.id)!;
      const core = squad.builtinSkills.map((name) => skillByName.get(name)!);
      const bytes = core.reduce(
        (sum, skill) =>
          sum +
          Buffer.byteLength(skill.content, "utf8") +
          skill.files
            .filter((file) => file.path.endsWith(".md"))
            .reduce((s, file) => s + Buffer.byteLength(file.content, "utf8"), 0),
        0,
      );
      expect(core.length).toBeLessThan(30);
      expect(bytes).toBeLessThan(coreCeilingKb(pack.id) * 1024);
    });
  }

  it("keeps each squad's core and role skills in the pack set", () => {
    for (const [id, squad] of squadConfigs) {
      expect(squad.roles.length, id).toBeGreaterThan(0);
    }
  });
});

describe("generated data is fresh", () => {
  it("matches what scripts/generate-kitviet-pack.mjs produces now", () => {
    const script = resolve(here, "../../../scripts/generate-kitviet-pack.mjs");
    const output = execFileSync(process.execPath, [script, "--check"], { encoding: "utf8" });
    expect(output).toContain("up to date");
  });
});
