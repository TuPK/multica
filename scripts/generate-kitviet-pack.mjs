#!/usr/bin/env node
// Generates packages/core/squads/kitviet-skills-data/*.ts, the Kit Viet skill and agent bundles the
// squad quickstart imports, from the vendored upstream in packages/core/squads/kitviet-upstream.
// Inputs: kitviet-rewrites.json (the rewrite table) and kitviet-patches/<skill>.json (exact-match
// patches). Run `pnpm generate:kitviet`; `--check` regenerates in memory and fails when the
// committed output is stale. The upstream is never read from the network.
//
// Order per skill or agent body: drop the Pro blocks, drop the platform sections, apply the patches
// (their `find` strings are upstream text, so they run before the rewrite table), apply the rewrite
// table, then rename skills the pack keeps under another name. Guards throw on leftovers.

import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { frontmatterDescription, skillFiles } from "./lib/skill-bundle.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const squadsDir = resolve(here, "../packages/core/squads");
const upstreamDir = join(squadsDir, "kitviet-upstream");
const patchesDir = join(squadsDir, "kitviet-patches");
const outDir = join(squadsDir, "kitviet-skills-data");

// Pack id, output file and the upstream trade folder it reads (null for the shared base).
export const PACKS = [
  { id: "common", file: "common", trade: null },
  { id: "admin", file: "admin", trade: "hanh-chinh-van-phong" },
  { id: "b2b", file: "b2b", trade: "kinh-doanh" },
  { id: "marketing", file: "marketing", trade: "marketing" },
  { id: "legal", file: "legal", trade: "phap-ly" },
  { id: "accounting", file: "accounting", trade: "ke-toan" },
];

// The lead replaces this agent (kv-squad-lead), so its instructions are not generated.
const EXCLUDED_AGENTS = new Set(["lap-ke-hoach-dieu-phoi"]);

// Text that must not survive into a generated body (spec 5.5 and F1, F3, F4, F7, F22).
const FORBIDDEN = [
  { re: /kv:pro(?![\w-])/, what: "Pro block reference" },
  { re: /mochistudio/i, what: "vendor advertising" },
  { re: /Bản Pro/, what: "Pro advertising" },
  { re: /\.kv-run/, what: "shared run folder" },
  { re: /CLAUDE\.md/, what: "CLAUDE.md profile" },
  { re: /AGENTS\.md/, what: "AGENTS.md profile" },
  { re: /ket-qua\//, what: "result folder" },
  { re: /phiên chính/i, what: "main session" },
  { re: /Claude Desktop/, what: "platform name" },
  { re: /Codex/, what: "platform name" },
  { re: /\d\d-[a-z-]+\.md/, what: "numbered stage file" },
];

const readJson = (path) => JSON.parse(readFileSync(path, "utf8"));

export function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Removes every block from `start` to `end` (inclusive). Unpaired markers fail.
export function removeBlocks(text, { start, end }) {
  const opens = text.split(start).length - 1;
  const closes = text.split(end).length - 1;
  if (opens !== closes) throw new Error(`unpaired block ${start} (${opens} starts, ${closes} ends)`);
  return text.replace(new RegExp(`${escapeRegExp(start)}[\\s\\S]*?${escapeRegExp(end)}\\n?`, "g"), "");
}

// Removes a level-2 section whose heading starts with `title`, up to the next level-2 heading.
export function removeSection(text, title) {
  const re = new RegExp(`^## ${escapeRegExp(title)}[^\\n]*\\n[\\s\\S]*?(?=^## |(?![\\s\\S]))`, "m");
  return text.replace(re, "");
}

// Applies exact-match patches. A `find` must occur exactly once in each file it applies to (more
// than once fails); `hitCounts[i]` counts the files patch i changed, so a skill can check it matched.
export function applyPatches(text, patches, label, hitCounts) {
  let out = text;
  patches.forEach(({ find, replace }, index) => {
    const count = out.split(find).length - 1;
    if (count > 1) throw new Error(`${label}: patch find matched ${count} times in one file: ${find.slice(0, 90)}`);
    if (count === 1) {
      out = out.replace(find, () => replace);
      hitCounts[index] += 1;
    }
  });
  return out;
}

// Applies the rewrite table in order and counts the matches of each rule into `hits`.
export function applyRules(text, rules, hits) {
  let out = text;
  for (const rule of rules) {
    const re = new RegExp(rule.pattern, rule.flags);
    out = out.replace(re, (...args) => {
      hits.set(rule.name, (hits.get(rule.name) ?? 0) + 1);
      return expandReplacement(rule.replace, args);
    });
  }
  return out;
}

// `$1`-style groups in a rule replacement, without the `$&`/`$$` special cases the table never uses.
function expandReplacement(template, args) {
  return template.replace(/\$(\d)/g, (_, n) => args[Number(n)] ?? "");
}

// Renames `kv:<from>` references and the skill itself, e.g. the Legal copy of soan-don-tu.
export function renameReferences(text, renames) {
  let out = text;
  for (const [from, to] of Object.entries(renames)) {
    out = out.replace(new RegExp(`kv:${escapeRegExp(from)}(?![\\w-])`, "g"), `kv:${to}`);
  }
  return out;
}

// Replaces the first non-empty line under the verdict heading with the one VERDICT line (F18).
// Gate and handover agents only; their template wording differs (QUA/CHẶN, Đạt/Chưa đạt, ...).
export function applyVerdictTemplate(text, section, line) {
  const lines = text.split("\n");
  const heading = lines.findIndex((value) => value.trim() === `## ${section}`);
  if (heading < 0) return text;
  let at = heading + 1;
  while (at < lines.length && lines[at].trim() === "") at += 1;
  if (at < lines.length && !lines[at].startsWith("#")) lines[at] = line;
  return lines.join("\n");
}

// The VERDICT lines of a body; a gate or handover must have exactly one.
export function verdictLines(body) {
  return body.split("\n").filter((line) => line.startsWith("VERDICT:"));
}

// The upstream writes double-quoted YAML, so an inner quote is `\"`; the bundle stores the plain text.
export function unescapeYamlQuotes(value) {
  return value.replace(/\\"/g, '"');
}

// Splits an agent or skill file into its frontmatter fields and body.
export function splitFrontmatter(text, name) {
  const match = /^---\n([\s\S]*?)\n---\n?/.exec(text);
  if (!match) throw new Error(`${name} has no frontmatter`);
  const fields = {};
  for (const line of match[1].split("\n")) {
    const colon = line.indexOf(":");
    if (colon > 0 && !/^\s/.test(line)) fields[line.slice(0, colon).trim()] = line.slice(colon + 1).trim();
  }
  return { fields, body: text.slice(match[0].length), frontmatterLines: match[1].split("\n") };
}

// Classifies an agent into the loop roles of spec 5.1 from its tools and description.
export function classifyAgent(tools, description) {
  const list = tools.split(",").map((tool) => tool.trim());
  if (list.includes("Write") && list.includes("Edit")) return "drafter";
  if (list.includes("Write")) return "handover";
  if (/^Gác cổng/.test(description)) return "gate";
  return "collector";
}

// Transforms one text file (skill body, reference file, or agent body) with the rewrite table.
export function transformText(text, { rewrites, patches, patchHits, renames, label, hits }) {
  let out = text;
  for (const block of rewrites.removeBlocks) out = removeBlocks(out, block);
  for (const title of rewrites.removeSections) out = removeSection(out, title);
  out = applyPatches(out, patches, label, patchHits);
  out = applyRules(out, rewrites.rules, hits);
  out = renameReferences(out, renames);
  return out;
}

// Leftover checks collect every violation so one run lists them all; buildPacks throws at the end.
let violations = [];

function guard(text, label) {
  const lines = text.split("\n");
  for (const { re, what } of FORBIDDEN) {
    lines.forEach((line, index) => {
      if (re.test(line)) violations.push(`${label}:${index + 1} still contains ${what}: ${line.trim().slice(0, 160)}`);
    });
  }
}

function listFolders(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

function listAgentFiles(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
    .map((entry) => entry.name)
    .sort();
}

// Loads the patch file for one skill (or agent) name, or nothing.
function patchesFor(name) {
  const path = join(patchesDir, `${name}.json`);
  return existsSync(path) ? readJson(path) : [];
}

export function buildPacks() {
  const rewrites = readJson(join(squadsDir, "kitviet-rewrites.json"));
  const upstream = readJson(join(upstreamDir, "UPSTREAM.json"));
  for (const file of ["LICENSE", "NOTICE.md", "UPSTREAM.json"]) {
    if (!existsSync(join(upstreamDir, file))) throw new Error(`kitviet-upstream/${file} is missing`);
  }
  const config = { source: "kitviet", upstream: upstream.repo, ref: upstream.ref, license: upstream.license };
  const hits = new Map();
  violations = [];
  const usedPatchFiles = new Set();
  const allPatches = readJson(join(patchesDir, "_all.json"));
  const allPatchHits = allPatches.map(() => 0);
  const packs = new Map();
  // The dropped skills and the renames are per pack; a skill folder name is unique across packs.
  for (const pack of PACKS) {
    const renames = pack.trade ? (rewrites.renamedSkills[pack.trade] ?? {}) : {};
    const skills = [];
    const agents = [];
    // The shared base is generated once, into the common pack; trade packs read only their trade folder.
    if (pack.id === "common") {
      for (const folder of listFolders(join(upstreamDir, "common", "skills"))) {
        if (rewrites.droppedSkills.includes(folder)) continue;
        skills.push(buildSkill({ folder, dir: join(upstreamDir, "common", "skills", folder), renames, rewrites, config, hits, usedPatchFiles, allPatches, allPatchHits }));
      }
      for (const file of listAgentFiles(join(upstreamDir, "common", "agents"))) {
        const name = file.replace(/\.md$/, "");
        if (EXCLUDED_AGENTS.has(name)) continue;
        agents.push(buildAgent({ name, path: join(upstreamDir, "common", "agents", file), renames, rewrites, hits, usedPatchFiles, allPatches, allPatchHits }));
      }
    } else {
      for (const folder of listFolders(join(upstreamDir, pack.trade, "skills"))) {
        if (rewrites.droppedSkills.includes(folder)) continue;
        skills.push(buildSkill({ folder, dir: join(upstreamDir, pack.trade, "skills", folder), renames, rewrites, config, hits, usedPatchFiles, allPatches, allPatchHits }));
      }
      for (const file of listAgentFiles(join(upstreamDir, pack.trade, "agents"))) {
        const name = file.replace(/\.md$/, "");
        if (EXCLUDED_AGENTS.has(name)) continue;
        agents.push(buildAgent({ name, path: join(upstreamDir, pack.trade, "agents", file), renames, rewrites, hits, usedPatchFiles, allPatches, allPatchHits }));
      }
    }
    packs.set(pack.id, { pack, skills, agents });
  }
  if (violations.length > 0) {
    throw new Error(`${violations.length} leftovers:\n${violations.slice(0, 80).join("\n")}`);
  }
  // Guards on the whole build.
  for (const name of readdirSync(patchesDir)) {
    if (!name.endsWith(".json") || name === "_all.json") continue;
    if (!usedPatchFiles.has(name.replace(/\.json$/, ""))) {
      throw new Error(`kitviet-patches/${name} matches no skill or agent`);
    }
  }
  allPatches.forEach((patch, index) => {
    if (allPatchHits[index] === 0) throw new Error(`_all.json patch matched nothing: ${patch.find.slice(0, 90)}`);
  });
  for (const rule of rewrites.rules) {
    if (!hits.has(rule.name)) throw new Error(`rewrite rule ${rule.name} matched nothing (stale rule)`);
  }
  return packs;
}

function buildSkill({ folder, dir, renames, rewrites, config, hits, usedPatchFiles, allPatches, allPatchHits }) {
  const original = readFileSync(join(dir, "SKILL.md"), "utf8");
  const short = renames[folder] ?? folder;
  const name = `kv:${short}`;
  const { frontmatterLines, body } = splitFrontmatter(original, folder);
  const label = `skill ${folder}`;
  // The frontmatter (description included) goes through the rewrite table too, then the name is set.
  const rewrittenFrontmatter = renameReferences(applyRules(frontmatterLines.join("\n"), rewrites.rules, hits), renames);
  const description = unescapeYamlQuotes(frontmatterDescription(rewrittenFrontmatter, folder));
  if (!description) throw new Error(`${folder}/SKILL.md has an empty description`);
  usedPatchFiles.add(folder);
  const ownPatches = patchesFor(folder);
  const ownHits = ownPatches.map(() => 0);
  const allTexts = [];
  const transform = (text, lab) => {
    const local = transformText(text, { rewrites, patches: ownPatches, patchHits: ownHits, renames, label: lab, hits });
    return applyAllPatches(local, allPatches, allPatchHits, lab);
  };
  const frontmatter = rewrittenFrontmatter
    .split("\n")
    .map((line) => (line.startsWith("name:") ? `name: ${short}` : line))
    .join("\n");
  const content = `---\n${frontmatter}\n---\n${transform(body, label)}`;
  guard(content, `${label}/SKILL.md`);
  allTexts.push({ path: "SKILL.md", content });
  for (const file of skillFiles(dir).sort()) {
    const raw = readFileSync(join(dir, file), "utf8");
    const transformed = file.endsWith(".md") ? transform(raw, `${label}/${file}`) : raw;
    if (file.endsWith(".md")) guard(transformed, `${label}/${file}`);
    allTexts.push({ path: file, content: transformed });
  }
  ownHits.forEach((count, index) => {
    if (count === 0) violations.push(`skill ${folder}: patch ${index + 1} matched no file: ${ownPatches[index].find.slice(0, 90)}`);
  });
  const skillFile = allTexts.find((file) => file.path === "SKILL.md");
  return {
    name,
    description,
    content: skillFile.content,
    files: allTexts.filter((file) => file.path !== "SKILL.md"),
    config,
  };
}

function buildAgent({ name, path, renames, rewrites, hits, usedPatchFiles, allPatches, allPatchHits }) {
  const original = readFileSync(path, "utf8");
  const label = `agent ${name}`;
  usedPatchFiles.add(name);
  const { fields, body, frontmatterLines } = splitFrontmatter(original, name);
  const description = unescapeYamlQuotes(
    frontmatterDescription(renameReferences(applyRules(frontmatterLines.join("\n"), rewrites.rules, hits), renames), name),
  );
  if (/\d\d-[a-z-]+\.md|kv:pro|\.kv-run|CLAUDE\.md|AGENTS\.md/.test(description)) {
    violations.push(`agent ${name}: description still contains a leftover: ${description.slice(0, 160)}`);
  }
  const role = classifyAgent(fields.tools ?? "", description);
  const ownPatches = patchesFor(name);
  const ownHits = ownPatches.map(() => 0);
  let text = transformText(body, { rewrites, patches: ownPatches, patchHits: ownHits, renames, label, hits });
  ownHits.forEach((count, index) => {
    if (count === 0) violations.push(`agent ${name}: patch ${index + 1} matched nothing: ${ownPatches[index].find.slice(0, 90)}`);
  });
  text = applyAllPatches(text, allPatches, allPatchHits, label);
  if (role === "gate" || role === "handover") {
    text = applyVerdictTemplate(text, rewrites.verdictSection, rewrites.verdictLine);
  }
  guard(text, label);
  if (role === "gate" || role === "handover") {
    const verdicts = verdictLines(text);
    if (verdicts.length !== 1) violations.push(`${label} (${role}) must have exactly one VERDICT line, found ${verdicts.length}`);
  }
  return {
    name,
    description,
    role,
    tools: (fields.tools ?? "").split(",").map((tool) => tool.trim()).filter(Boolean),
    instructions: text.trim(),
  };
}

// `_all.json` patches apply to every text file; each may match at most once per file.
function applyAllPatches(text, allPatches, allPatchHits, label) {
  let out = text;
  allPatches.forEach((patch, index) => {
    const count = out.split(patch.find).length - 1;
    if (count > 1) throw new Error(`${label}: _all patch matched ${count} times`);
    if (count === 1) {
      out = out.replace(patch.find, () => patch.replace);
      allPatchHits[index] += 1;
    }
  });
  return out;
}

// Every pack module exports the same two names, so kitviet-packs.ts can load any of them.
export function renderPack(pack, entry) {
  const skills = JSON.stringify(entry.skills, null, 2);
  const agents = JSON.stringify(entry.agents, null, 2);
  return `${HEADER}
import type { KitvietAgent, KitvietSkill } from "./types";

/** Skills of the ${pack.id} pack. The quickstart loads this file on demand. */
export const KITVIET_BUILTIN_SKILLS: readonly KitvietSkill[] = ${skills};

/** Agents of the ${pack.id} pack, classified by loop role. */
export const KITVIET_AGENTS: readonly KitvietAgent[] = ${agents};
`;
}

const HEADER = `// Generated by scripts/generate-kitviet-pack.mjs from packages/core/squads/kitviet-upstream.
// Do not edit by hand; run \`pnpm generate:kitviet\`.`;

const TYPES = `${HEADER}

export type { BuiltinSkill, BuiltinSkillFile } from "../sdlc-skills-data";
import type { BuiltinSkill } from "../sdlc-skills-data";

/** Where a Kit Viet skill came from; a separate source from SDLC_SKILL_SOURCE. */
export interface KitvietSkillSource {
  source: "kitviet";
  upstream: string;
  ref: string;
  license: string;
}

export interface KitvietSkill extends BuiltinSkill {
  config: KitvietSkillSource;
}

/** The loop role an agent plays (spec 5.1). \`lead\` is written by hand, not generated. */
export type KitvietRole = "collector" | "drafter" | "gate" | "handover";

export interface KitvietAgent {
  name: string;
  description: string;
  role: KitvietRole;
  tools: readonly string[];
  instructions: string;
}
`;

// Every generated file as { path, content }, keyed by its file name in kitviet-skills-data.
export function generateOutputs() {
  const packs = buildPacks();
  const outputs = new Map([["types.ts", TYPES]]);
  for (const pack of PACKS) {
    const { skills, agents } = packs.get(pack.id);
    outputs.set(`${pack.file}.ts`, renderPack(pack, { skills, agents }));
  }
  return outputs;
}

function main() {
  const check = process.argv.includes("--check");
  const outputs = generateOutputs();
  if (check) {
    const stale = [...outputs].filter(([file, content]) => {
      const path = join(outDir, file);
      return !existsSync(path) || readFileSync(path, "utf8") !== content;
    });
    if (stale.length > 0) {
      console.error(`stale kitviet pack data: ${stale.map(([file]) => file).join(", ")}; run pnpm generate:kitviet`);
      process.exit(1);
    }
    console.log(`kitviet pack data is up to date (${outputs.size} files)`);
    return;
  }
  mkdirSync(outDir, { recursive: true });
  for (const [file, content] of outputs) writeFileSync(join(outDir, file), content);
  console.log(`wrote ${outputs.size} files to ${outDir}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main();
