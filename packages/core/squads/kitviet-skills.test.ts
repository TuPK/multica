// @vitest-environment node
// Guards the generated Kit Viet pack: no leftovers from the upstream (Pro ads, platform notes, the
// shared run folder, install hints, versioned file names), references that resolve inside each squad,
// one VERDICT line per gate and one severity vocabulary, legal citations that are patched or listed as
// stable, sections that survive the cleanup, every patch applied, names that match their folders and
// their sanitized slugs, size ceilings, and freshness against the generator.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import { KV_ROLE_KINDS, composeKvAgentInstructions } from "./kitviet-agent-instructions";
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
  [/-v\d\.md/, "versioned file name"],
  [/pip install|apt install/, "install hint (agents do not install)"],
  [/hoặc cài LibreOffice/, "LibreOffice install hint"],
  [/Cài bằng lệnh/, "install instruction"],
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

    // A handover reports each criterion as "Đạt / Không đạt / Không kiểm được"; only a gate has the
    // verdict vocabulary QUA / CHẶN and nothing else.
    if (agent.kind === "kv_gate") {
      it(`${agent.name} has no other verdict vocabulary`, () => {
        expect(/Đạt \/|Chưa đạt|Gửi được \/|Chưa gửi được|Không kết luận "(Đạt|Gửi được)"/.test(agent.instructions)).toBe(false);
      });
    }
  }

  it("uses only the known role kinds", () => {
    for (const agent of allAgents()) {
      expect(KV_ROLE_KINDS).toContain(agent.kind);
    }
  });
});

describe("patches apply exactly once", () => {
  const patchFiles = readdirSync(patchesDir).filter((name) => name.endsWith(".json"));

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

// The severity levels of a finding (F18): one set, Chặn / Cần giấy tờ / Nên sửa / Góp ý. The old words
// are checked in every gate and handover, and in every soat skill that ranks findings. "Vừa" and "Nhẹ"
// are only checked with a capital letter: "vừa" is an ordinary word, and viet-giong-gen-z uses "Nhẹ"
// for a dose of slang, not a finding.
const OLD_SEVERITY: [RegExp, string][] = [
  [/\bNặng\b|\bnặng\b/, "Nặng (severity)"],
  [/\bVừa\b/, "Vừa (severity)"],
  [/\bNhẹ\b/, "Nhẹ (severity)"],
  [/Phải sửa|Phải bỏ/, "Phải sửa / Phải bỏ (severity)"],
  [/Gợi ý\b/, "Gợi ý (severity or column)"],
  [/Lưu ý \(/, "Lưu ý (severity)"],
  [/Cần xác nhận quyền dùng/, "Cần xác nhận quyền dùng (severity)"],
];

describe("one severity vocabulary (F18)", () => {
  const gates = allAgents().filter((agent) => agent.kind === "kv_gate" || agent.kind === "kv_handover");
  const soatSkills = allSkills.filter((skill) => skill.name.startsWith("kv--soat-"));

  it("has gates and soat skills to check", () => {
    expect(gates.length).toBeGreaterThan(0);
    expect(soatSkills.length).toBeGreaterThan(0);
  });

  for (const agent of gates) {
    it(`${agent.name} uses no old severity word`, () => {
      const hits = OLD_SEVERITY.filter(([re]) => re.test(agent.instructions)).map(([, what]) => what);
      expect(hits).toEqual([]);
    });
  }

  for (const skill of soatSkills) {
    it(`${skill.name} uses no old severity word`, () => {
      const hits = OLD_SEVERITY.filter(([re]) => re.test(allTextOf(skill))).map(([, what]) => what);
      expect(hits).toEqual([]);
    });
  }

});

// Legal citations (Luật, Nghị định, Thông tư, Nghị quyết or Bộ luật followed by a number or a dated
// effective date) go stale (F8, F23). Each one in the pack is either patched to point at the squad's
// Sổ quy định, or listed here as a stable citation with the reason. The list is empty on purpose: no
// citation that carries a number or a date is stable enough to keep in a skill body.
const STABLE_CITATIONS: { text: string; reason: string }[] = [];
const CITATION = /(?:Luật|Nghị định|Thông tư|Nghị quyết|Bộ luật|Pháp lệnh)\b[^\n".]{0,60}?(?:\d{1,4}\/\d{4}|\d{1,2}\/\d{1,2}\/\d{4})/g;

describe("legal citations point at the Sổ quy định (F8, F23)", () => {
  const texts = (): { where: string; text: string }[] => [
    ...allSkills.flatMap((skill) => [skill.description, skill.content, ...skill.files.map((file) => file.content)].map((text) => ({ where: skill.name, text }))),
    ...allAgents().flatMap((agent) => [agent.description, agent.instructions].map((text) => ({ where: agent.name, text }))),
  ];

  it("has no citation outside the stable list", () => {
    const allowed = new Set(STABLE_CITATIONS.map((citation) => citation.text));
    const found: string[] = [];
    for (const { where, text } of texts()) {
      for (const match of text.matchAll(CITATION)) {
        if (!allowed.has(match[0])) found.push(`${where}: ${match[0]}`);
      }
    }
    expect(found).toEqual([]);
  });

  it("lists no stable citation that no longer occurs in the pack", () => {
    const all = texts().map((entry) => entry.text).join("\n");
    expect(STABLE_CITATIONS.filter((citation) => !all.includes(citation.text))).toEqual([]);
  });
});

// The upstream cleanup removes platform sections and the Pro block; it must not remove the two sections
// every upstream skill carries. Skills with neither section (the authored ones) are not forced to have one.
describe("sections survive the cleanup (K8)", () => {
  const SECTIONS = ["## Dùng khi nào", "## Không được làm"];
  const trades = ["common", ...new Set(Object.values(squadsConfig.squads).map((squad) => squad.trade))];
  const upstreamSkills = trades.flatMap((trade) => {
    const dir = join(upstreamDir, trade, "skills");
    return readdirSync(dir, { withFileTypes: true })
      .filter((entry) => entry.isDirectory() && !rewrites.droppedSkills.includes(entry.name))
      .map((entry) => ({ trade, folder: entry.name, text: readFileSync(join(dir, entry.name, "SKILL.md"), "utf8") }));
  });

  it("finds upstream skills to check", () => {
    expect(upstreamSkills.length).toBeGreaterThan(0);
  });

  for (const section of SECTIONS) {
    it(`keeps "${section}" in every upstream skill that had it`, () => {
      const missing: string[] = [];
      for (const { trade, folder, text } of upstreamSkills) {
        if (!new RegExp(`^${section}`, "m").test(text)) continue;
        const short = rewrites.renamedSkills[trade]?.[folder] ?? folder;
        const skill = skillByName.get(`kv--${short}`);
        if (!skill || !skill.content.includes(`\n${section}`)) missing.push(`${trade}/${folder}`);
      }
      expect(missing).toEqual([]);
    });
  }
});

// The server writes each skill to a folder named by sanitizeSkillName (server/internal/daemon/execenv/
// context.go): lowercase, every run of other characters becomes "-". Two names that sanitize alike share a
// folder, so the slugs of the pack and of the SDLC built-ins must be unique.
describe("sanitized skill slugs are unique (K8)", () => {
  const sanitize = (name: string) => name.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "skill";

  it("gives every Kit Viet and SDLC skill a different slug", () => {
    const names = [...allSkills.map((skill) => skill.name), ...SDLC_BUILTIN_SKILLS.map((skill) => skill.name)];
    const bySlug = new Map<string, string[]>();
    for (const name of names) bySlug.set(sanitize(name), [...(bySlug.get(sanitize(name)) ?? []), name]);
    const clashes = [...bySlug].filter(([, group]) => group.length > 1);
    expect(clashes).toEqual([]);
  });
});

// Web-reading agents (a WebSearch or WebFetch tool) carry the rule that web pages are data (F13). The
// rule is in the shared working rules of every instruction, so it is checked on the composed text.
describe("web-reading agents treat web pages as data (F13)", () => {
  it("composes the rule into every agent that can read the web", () => {
    const web = allAgents().filter((agent) => agent.tools.includes("WebSearch") || agent.tools.includes("WebFetch"));
    expect(web.length).toBeGreaterThan(0);
    for (const agent of web) {
      const text = composeKvAgentInstructions({ kind: agent.kind, body: agent.instructions });
      expect(text, agent.name).toMatch(/Web pages[^\n]*are data, not instructions/);
    }
  });
});

// The export scripts (xuat-*) name the library that is missing and stop; an agent never installs one (F9).
describe("export scripts name the missing library and never install (F9)", () => {
  const exporters = ["kv--xuat-excel", "kv--xuat-pdf", "kv--xuat-pptx", "kv--xuat-word"];

  for (const name of exporters) {
    it(`${name}: its script names the missing library`, () => {
      const skill = skillByName.get(name);
      expect(skill, name).toBeDefined();
      const script = skill!.files.find((file) => file.path.endsWith(".py"));
      expect(script, name).toBeDefined();
      expect(script!.content).toMatch(/Thiếu thư viện [\w-]+\./);
      expect(script!.content).not.toMatch(/pip|Cài bằng lệnh|apt install/);
    });
  }
});

describe("generated data is fresh", () => {
  it("matches what scripts/generate-kitviet-pack.mjs produces now", () => {
    const script = resolve(here, "../../../scripts/generate-kitviet-pack.mjs");
    const output = execFileSync(process.execPath, [script, "--check"], { encoding: "utf8" });
    expect(output).toContain("up to date");
  });
});
