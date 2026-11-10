#!/usr/bin/env node
// Generates packages/core/squads/kitviet-skills-data/*.ts, the Kit Viet skill, agent and squad data
// the squad quickstart imports, from the vendored upstream in packages/core/squads/kitviet-upstream.
// Inputs:
//   kitviet-rewrites.json   the rewrite table (Pro blocks, platform sections, file names, verdicts)
//   kitviet-patches/<name>.json  exact-match patches per skill or agent (upstream text)
//   kitviet-squads.json     per squad: trade folder, agent -> role key and kind, core skills,
//                           promoted skills, leader skills and role skills (spec 5.10, 6, F17)
//   kitviet-skills/<kv-*>/  authored skills (spec 5.5), shared by every squad
// Run `pnpm generate:kitviet`; `--check` regenerates in memory and fails when the committed output is
// stale. The upstream is never read from the network.
//
// Each squad has its own pack file (admin.ts, ...): its trade skills, its agents rendered with that
// squad's role keys and agent bodies. squads.ts holds the names and roles of every squad, with no text,
// so the template registry can import it. The shared skills (common.ts) are rendered
// once, with neutral wording for agent names, because every squad loads them.
//
// Reference rules (F17), applied per squad to every skill and agent body:
//   kv:<skill> in the squad's resolution set (core + roleSkills)  -> the bundle name kv--<skill>
//   kv:<skill> elsewhere                                          -> "nhờ lead tạo yêu cầu"
//   <agent> mapped to a role of the squad                         -> the role key
//   <agent> not in the squad                                      -> "nhờ lead"
// Guards throw (or list every violation) on leftovers, unknown references and stale data.

import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { frontmatterDescription, skillFiles } from "./lib/skill-bundle.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const squadsDir = resolve(here, "../packages/core/squads");
const upstreamDir = join(squadsDir, "kitviet-upstream");
const patchesDir = join(squadsDir, "kitviet-patches");
const authoredDir = join(squadsDir, "kitviet-skills");
const outDir = join(squadsDir, "kitviet-skills-data");

const readJson = (path) => JSON.parse(readFileSync(path, "utf8"));
const squadConfig = readJson(join(squadsDir, "kitviet-squads.json"));
const rewrites = readJson(join(squadsDir, "kitviet-rewrites.json"));
const upstream = readJson(join(upstreamDir, "UPSTREAM.json"));

// Squads in file order; each one is a pack file named after its id.
export const SQUADS = Object.entries(squadConfig.squads).map(([id, squad]) => ({ id, ...squad }));

// The shared pack, then one pack per squad. The file name is the pack id.
export const PACKS = [
  { id: "common", file: "common", trade: null },
  ...SQUADS.map((squad) => ({ id: squad.id, file: squad.id, trade: squad.trade })),
];

// What a reference outside the squad becomes (F17). Parenthesised so it reads inside a sentence.
const NO_AGENT = "(vai không có trong squad: nhờ lead)";
const NO_SKILL = "(việc ngoài squad: nhờ lead tạo yêu cầu)";
const ROLE_KIND_CLASS = {
  kv_lead: "lead",
  kv_collector: "collector",
  kv_drafter: "drafter",
  kv_gate: "gate",
  kv_handover: "handover",
};

// Text that must not survive into a generated body or script (spec 5.5 and F1, F3, F4, F7, F9, F22).
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
  { re: /-v\d\.md/, what: "versioned file name" },
  { re: /pip install|apt install/, what: "install hint (agents do not install)" },
  { re: /hoặc cài LibreOffice/, what: "LibreOffice install hint" },
  { re: /Cài bằng lệnh/, what: "install instruction" },
  { re: /kv:[a-z0-9-]/, what: "unresolved kv: reference" },
  // The authored skills are folders (kv-delivery-core); the bundle name has two dashes (kv--delivery-core).
  { re: /(?<![\w-])kv-(delivery-core|squad-lead|company-profile)/, what: "authored folder name instead of its bundle name" },
];

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

// Words of a role key that are written as acronyms, not capitalised: `b2b_lead` -> `B2B Lead`.
const ACRONYMS = new Map([["b2b", "B2B"]]);

// The role display name of a role key: `legal_reader` -> `Legal Reader`.
export function displayRole(role) {
  return role
    .split("_")
    .map((word) => ACRONYMS.get(word) ?? word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

// The indefinite article before a squad name: "an Office Admin", "a B2B Proposal Desk". A name that
// starts with a vowel letter takes "an". The squad names of this pack are checked by the test below
// (a name starting with an acronym read as a vowel would need a rule here).
export function indefiniteArticle(name) {
  return /^[aeiou]/i.test(name) ? "an" : "a";
}

// The squad header of a role: "You are the Admin Files in an Office Admin (Hành chính) squad, unattended."
// It is the only "You are" opener of an agent of that squad; the kind text starts with "Your job".
export function roleHeader(display, squadName) {
  return `You are the ${display} in ${indefiniteArticle(squadName)} ${squadName} squad, unattended.`;
}

// Leftover checks collect every violation so one run lists them all; buildPacks throws at the end.
let violations = [];
let ruleHits = new Map();

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

function listAgentNames(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
    .map((entry) => entry.name.replace(/\.md$/, ""))
    .sort();
}

// The patch list for one skill folder or agent name, or none.
function patchesFor(name) {
  const path = join(patchesDir, `${name}.json`);
  return existsSync(path) ? readJson(path) : [];
}

// The bundle name of an upstream skill: `kv:` written as `kv--` (spec 5.5).
function bundleName(short) {
  return `kv--${short}`;
}

// Shared skill keys and their bundle names: the base skills and the authored skills.
function authoredKey(folder) {
  return folder;
}

// Reference resolution for one body. `mode` is "squad" (one squad's wording) or "neutral" (shared
// skills, loaded by every squad, so agent names become generic phrases).
//   skills:       key -> bundle name, the skills this body may name
//   knownSkills:  every skill key in the pack set (a key outside `skills` is out of the squad)
//   roles:        upstream agent name -> role key (squad mode)
//   knownAgents:  every upstream agent name
export function resolveReferences(text, ctx, label) {
  const withSkills = text.replace(/(`?)kv:([a-z0-9-]+)(`?)/g, (match, open, ref, close) => {
    if (ctx.roles.has(ref)) return `${open}${ctx.roles.get(ref)}${close}`;
    if (ctx.knownAgents.has(ref)) return agentPhrase(ref, ctx);
    if (ctx.skills.has(ref)) return `${open}${ctx.skills.get(ref)}${close}`;
    if (ctx.knownSkills.has(ref)) {
      if (ctx.mode === "neutral") violations.push(`${label}: shared skill names kv:${ref}, which is not in the shared core`);
      return NO_SKILL;
    }
    violations.push(`${label}: unknown reference ${match}`);
    return match;
  });
  let out = withSkills;
  for (const name of ctx.agentNamesByLength) {
    const re = new RegExp(`(?<![\\w-])${escapeRegExp(name)}(?![\\w-])`, "g");
    out = out.replace(re, () => agentPhrase(name, ctx));
  }
  return out;
}

function agentPhrase(name, ctx) {
  // Squad mode: a mapped role key, else the lead request. Neutral mode: the shared phrase, else the lead request.
  if (ctx.roles.has(name)) return ctx.roles.get(name);
  if (ctx.mode === "neutral") return ctx.agentPhrases[name] ?? NO_AGENT;
  return NO_AGENT;
}

// Runs one markdown text (a body or one reference file) through the block and section removal, the
// patches, the rewrite table and the reference rules, then guards it. `hits` counts, per own patch,
// the texts it matched; the caller reports a patch that matched no text of its skill or agent.
function transformBody(text, { patches, hits, ctx, label }) {
  let out = text;
  for (const block of rewrites.removeBlocks) out = removeBlocks(out, block);
  for (const title of rewrites.removeSections) out = removeSection(out, title);
  out = applyPatches(out, patches, label, hits);
  out = applyRules(out, rewrites.rules, ruleHits);
  out = resolveReferences(out, ctx, label);
  guard(out, label);
  return out;
}

// A script of a skill (xuat-*): the same patches and rewrite rules as the prose, so an install hint
// in a message is reworded too (F9), then its references are resolved and it is guarded. Blocks and
// platform sections are prose-only and are not removed here.
function transformScript(text, { patches, hits, ctx, label }) {
  let out = applyPatches(text, patches, label, hits);
  out = applyRules(out, rewrites.rules, ruleHits);
  out = resolveReferences(out, ctx, label);
  guard(out, label);
  return out;
}

// Rebuilds the frontmatter: the bundle name and the resolved description, then the other fields.
function buildFrontmatter(lines, bundle, description) {
  const rest = [];
  let folded = false;
  for (const line of lines) {
    if (line.startsWith("description:")) {
      folded = true;
      continue;
    }
    if (folded && /^\s/.test(line)) continue;
    folded = false;
    if (line.startsWith("name:")) continue;
    rest.push(line);
  }
  return [`name: ${bundle}`, `description: ${JSON.stringify(description)}`, ...rest].join("\n");
}

// Renders one skill folder. `key` is the skill key; `bundle` is its name in the pack.
function renderSkill({ dir, key, bundle, ctx, config, label, files }) {
  const original = readFileSync(join(dir, "SKILL.md"), "utf8").replace(/\r\n/g, "\n");
  const { frontmatterLines, body } = splitFrontmatter(original, key);
  const ruled = applyRules(frontmatterLines.join("\n"), rewrites.rules, ruleHits).split("\n");
  const rawDescription = unescapeYamlQuotes(frontmatterDescription(ruled.join("\n"), key));
  if (!rawDescription) throw new Error(`${label} has an empty description`);
  const description = resolveReferences(rawDescription, ctx, `${label} description`);
  guard(description, `${label} description`);
  const ownPatches = patchesFor(key);
  const hits = ownPatches.map(() => 0);
  const content = transformBody(body, { patches: ownPatches, hits, ctx, label: `${label}/SKILL.md` });
  const skillBody = `---\n${buildFrontmatter(ruled, bundle, description)}\n---\n${content}`;
  const bundleFiles = [];
  for (const file of files) {
    const raw = readFileSync(join(dir, file), "utf8").replace(/\r\n/g, "\n");
    // Markdown goes through the whole table; a script goes through the patches and the rules (F9).
    const text = file.endsWith(".md")
      ? transformBody(raw, { patches: ownPatches, hits, ctx, label: `${label}/${file}` })
      : transformScript(raw, { patches: ownPatches, hits, ctx, label: `${label}/${file}` });
    guard(text, `${label}/${file}`);
    bundleFiles.push({ path: file, content: text });
  }
  reportUnmatchedPatches(ownPatches, hits, label);
  return { name: bundle, description, content: skillBody, files: bundleFiles, config };
}

// A patch that matched no text of its own skill or agent means upstream changed under it.
function reportUnmatchedPatches(patches, hits, label) {
  hits.forEach((count, index) => {
    if (count === 0) violations.push(`${label}: patch ${index + 1} matched nothing: ${patches[index].find.slice(0, 90)}`);
  });
}

// Renders an agent's body and frontmatter fields for one role source.
function renderAgentSource(source, ctx) {
  const original = readFileSync(source.path, "utf8").replace(/\r\n/g, "\n");
  const { fields, body, frontmatterLines } = splitFrontmatter(original, source.upstream);
  const description = unescapeYamlQuotes(frontmatterDescription(frontmatterLines.join("\n"), source.upstream));
  const label = `agent ${source.upstream}`;
  const ownPatches = patchesFor(source.upstream);
  const hits = ownPatches.map(() => 0);
  const text = transformBody(body, { patches: ownPatches, hits, ctx, label });
  reportUnmatchedPatches(ownPatches, hits, label);
  const ruledDescription = resolveReferences(
    applyRules(description, rewrites.rules, ruleHits),
    ctx,
    `${label} description`,
  );
  guard(ruledDescription, `${label} description`);
  const tools = (fields.tools ?? "").split(",").map((tool) => tool.trim()).filter(Boolean);
  return { description: ruledDescription, rawDescription: description, tools, body: text.trim() };
}

// The per-squad context: which skills a body may name, and the agent role keys of the squad.
export function buildContext({ mode, skills, knownSkills, roles, knownAgents, agentPhrases }) {
  const agentNamesByLength = [...knownAgents].sort((a, b) => b.length - a.length);
  return { mode, skills, knownSkills, roles, knownAgents, agentNamesByLength, agentPhrases };
}

// Loads the upstream folders once: skill folders by key, agent files by name, per pack.
function loadUpstream() {
  const dropped = new Set(rewrites.droppedSkills);
  const commonSkills = new Map();
  for (const folder of listFolders(join(upstreamDir, "common", "skills"))) {
    if (dropped.has(folder)) continue;
    commonSkills.set(folder, { dir: join(upstreamDir, "common", "skills", folder) });
  }
  const tradeSkills = new Map();
  const tradeAgents = new Map();
  for (const squad of SQUADS) {
    const skills = new Map();
    for (const folder of listFolders(join(upstreamDir, squad.trade, "skills"))) {
      if (dropped.has(folder)) continue;
      skills.set(folder, { dir: join(upstreamDir, squad.trade, "skills", folder) });
    }
    tradeSkills.set(squad.trade, skills);
    const agents = new Map();
    for (const name of listAgentNames(join(upstreamDir, squad.trade, "agents"))) {
      agents.set(name, join(upstreamDir, squad.trade, "agents", `${name}.md`));
    }
    tradeAgents.set(squad.trade, agents);
  }
  const commonAgents = new Map();
  for (const name of listAgentNames(join(upstreamDir, "common", "agents"))) {
    commonAgents.set(name, join(upstreamDir, "common", "agents", `${name}.md`));
  }
  const authored = new Map();
  for (const folder of listFolders(authoredDir)) {
    authored.set(authoredKey(folder), { dir: join(authoredDir, folder), folder });
  }
  return { dropped, commonSkills, tradeSkills, tradeAgents, commonAgents, authored };
}

// Every skill key the pack set knows (any trade, shared or authored), for "outside the squad" checks.
function allSkillKeys(up) {
  const keys = new Set([...up.commonSkills.keys(), ...up.authored.keys()]);
  for (const skills of up.tradeSkills.values()) for (const key of skills.keys()) keys.add(key);
  return keys;
}

function allAgentNames(up) {
  const names = new Set(up.commonAgents.keys());
  for (const agents of up.tradeAgents.values()) for (const name of agents.keys()) names.add(name);
  return names;
}

// The skill keys a squad owns for resolution: shared, its own trade, and the renames of its trade.
function squadSkillBundles(up, squad) {
  const renames = rewrites.renamedSkills[squad.trade] ?? {};
  const map = new Map();
  for (const key of up.commonSkills.keys()) map.set(key, bundleName(key));
  for (const key of up.authored.keys()) map.set(key, bundleName(key.replace(/^kv-/, "")));
  for (const key of up.tradeSkills.get(squad.trade).keys()) map.set(key, bundleName(renames[key] ?? key));
  return map;
}

// The shared skill keys: base skills and the authored core skill, in the file's order.
function sharedKeys() {
  return squadConfig.sharedBuiltinSkills;
}

// Maps each role of a squad to its sources (agents in mapping order) and the role kind.
function squadRoles(squad, up) {
  const roles = new Map();
  const roleOf = new Map();
  for (const [name, spec] of Object.entries(squad.agents)) {
    const path = up.tradeAgents.get(squad.trade).get(name) ?? up.commonAgents.get(name);
    if (!path) {
      violations.push(`${squad.id}: agent ${name} is in neither ${squad.trade} nor the shared agents`);
      continue;
    }
    if (!ROLE_KIND_CLASS[spec.kind]) violations.push(`${squad.id}: ${name} has an unknown kind ${spec.kind}`);
    if (!roles.has(spec.role)) roles.set(spec.role, { kind: spec.kind, sources: [] });
    const role = roles.get(spec.role);
    if (role.kind !== spec.kind) violations.push(`${squad.id}: role ${spec.role} has two kinds`);
    role.sources.push({ upstream: name, path, kind: spec.kind });
    roleOf.set(name, spec.role);
  }
  return { roles, roleOf };
}

function buildPacks() {
  violations = [];
  ruleHits = new Map();
  for (const file of ["LICENSE", "NOTICE.md", "UPSTREAM.json"]) {
    if (!existsSync(join(upstreamDir, file))) throw new Error(`kitviet-upstream/${file} is missing`);
  }
  const config = { source: "kitviet", upstream: upstream.repo, ref: upstream.ref, license: upstream.license };
  const authoredConfig = { source: "kitviet", upstream: "authored in this repository", ref: "authored", license: "repository" };
  const up = loadUpstream();
  const knownSkills = allSkillKeys(up);
  const knownAgents = allAgentNames(up);
  const usedPatchFiles = new Set();
  const packs = new Map();

  // The shared pack: base skills and authored skills, in neutral wording. Every authored skill must
  // be shared or a leader skill of some squad, and every shared key must exist.
  const sharedBundles = new Map([
    ...[...up.commonSkills.keys()].map((key) => [key, bundleName(key)]),
    ...[...up.authored.keys()].map((key) => [key, bundleName(key.replace(/^kv-/, ""))]),
  ]);
  const leaderKeys = new Set(SQUADS.flatMap((squad) => squad.leaderSkills));
  for (const key of up.commonSkills.keys()) {
    if (!sharedKeys().includes(key)) violations.push(`common skill ${key} is not in sharedBuiltinSkills (orphan)`);
  }
  for (const key of up.authored.keys()) {
    if (!sharedKeys().includes(key) && !leaderKeys.has(key)) violations.push(`authored skill ${key} is used by no squad`);
  }
  for (const key of sharedKeys()) {
    if (!sharedBundles.has(key)) violations.push(`kitviet-squads.json names shared skill ${key}, which does not exist`);
  }
  const sharedCtx = buildContext({
    mode: "neutral",
    skills: new Map([...sharedBundles].filter(([key]) => sharedKeys().includes(key))),
    knownSkills,
    roles: new Map(),
    knownAgents,
    agentPhrases: squadConfig.sharedAgentPhrases,
  });
  const commonSkills = [];
  for (const key of [...up.commonSkills.keys()].sort()) {
    const entry = up.commonSkills.get(key);
    usedPatchFiles.add(key);
    commonSkills.push(
      renderSkill({ dir: entry.dir, key, bundle: bundleName(key), ctx: sharedCtx, config, label: `skill ${key}`, files: skillFiles(entry.dir).sort() }),
    );
  }
  for (const [key, entry] of [...up.authored.entries()].sort()) {
    commonSkills.push(
      renderSkill({
        dir: entry.dir,
        key,
        bundle: bundleName(key.replace(/^kv-/, "")),
        ctx: sharedCtx,
        config: authoredConfig,
        label: `authored ${key}`,
        files: skillFiles(entry.dir).sort(),
      }),
    );
  }
  packs.set("common", { skills: commonSkills, agents: [], squad: null });

  // One pack per squad.
  for (const squad of SQUADS) {
    const bundles = squadSkillBundles(up, squad);
    const { roles, roleOf } = squadRoles(squad, up);
    const core = [...sharedKeys(), ...squad.builtinSkills];
    const roleSkillKeys = Object.values(squad.roleSkills ?? {}).flat();
    const resolution = new Set([...core, ...roleSkillKeys]);
    for (const key of resolution) {
      if (!bundles.has(key)) violations.push(`${squad.id}: ${key} is in neither the shared pack nor the ${squad.trade} trade`);
    }
    for (const key of squad.builtinSkills) {
      if (!up.tradeSkills.get(squad.trade).has(key)) violations.push(`${squad.id}: builtinSkills names ${key}, which is not in ${squad.trade}`);
    }
    for (const key of squad.promoted ?? []) {
      if (!squad.builtinSkills.includes(key)) violations.push(`${squad.id}: promoted ${key} is not in builtinSkills`);
    }
    for (const key of squad.leaderSkills) {
      if (!up.authored.has(key)) violations.push(`${squad.id}: leaderSkills names ${key}, which is not an authored skill`);
    }
    for (const [role, keys] of Object.entries(squad.roleSkills ?? {})) {
      if (!roles.has(role)) violations.push(`${squad.id}: roleSkills names ${role}, which is not a role of the squad`);
      if (keys.length > 6) violations.push(`${squad.id}: ${role} has ${keys.length} role skills (max 6)`);
    }
    if (roles.size > 9) violations.push(`${squad.id}: ${roles.size} roles (max 9 desks with the lead)`);

    // Orphan guard (spec 5.10, K8): every trade skill is attached to a role or the squad, or it is
    // deferred by name with the reason in kitviet-squads.json. A skill in neither is a leftover.
    const deferred = new Set(squad.deferredSkills ?? []);
    const tradeKeys = up.tradeSkills.get(squad.trade);
    for (const key of deferred) {
      if (!tradeKeys.has(key)) violations.push(`${squad.id}: deferredSkills names ${key}, which is not in ${squad.trade}`);
      if (resolution.has(key)) violations.push(`${squad.id}: ${key} is deferred and also attached`);
    }
    for (const key of tradeKeys.keys()) {
      if (!resolution.has(key) && !deferred.has(key)) violations.push(`${squad.id}: ${key} is attached to nothing and not deferred (orphan)`);
    }

    // The squad's trade skills are all rendered (core and extended); a body names only what the
    // squad resolves, so the rest of the trade reads as "nhờ lead tạo yêu cầu".
    const resolved = new Map();
    for (const key of resolution) resolved.set(key, bundles.get(key));
    const squadCtx = buildContext({
      mode: "squad",
      skills: resolved,
      knownSkills,
      roles: roleOf,
      knownAgents,
      agentPhrases: {},
    });
    const skills = [];
    for (const key of [...up.tradeSkills.get(squad.trade).keys()].sort()) {
      const entry = up.tradeSkills.get(squad.trade).get(key);
      usedPatchFiles.add(key);
      skills.push(
        renderSkill({
          dir: entry.dir,
          key,
          bundle: bundles.get(key),
          ctx: squadCtx,
          config,
          label: `skill ${squad.trade}/${key}`,
          files: skillFiles(entry.dir).sort(),
        }),
      );
    }

    const agents = [];
    const roleEntries = [];
    for (const [role, role0] of roles) {
      const display = displayRole(role);
      const header = roleHeader(display, squad.name);
      if (role0.kind === "kv_lead") {
        // The lead has no upstream agent, so it has no body; its squad notes are written by hand.
        roleEntries.push({ key: role, kind: role0.kind, name: display, header, hasBody: false });
        continue;
      }
      const rendered = role0.sources.map((source) => {
        usedPatchFiles.add(source.upstream);
        return { source, ...renderAgentSource(source, squadCtx) };
      });
      for (const item of rendered) {
        const expected = ROLE_KIND_CLASS[role0.kind];
        const actual = classifyAgent(item.tools.join(", "), item.rawDescription);
        if (actual !== expected) violations.push(`${squad.id}: ${item.source.upstream} is a ${actual} by its tools, but the squad maps it as ${expected}`);
      }
      let text = rendered.map((item) => item.body).join("\n\n---\n\n");
      if (role0.kind === "kv_gate" || role0.kind === "kv_handover") {
        text = applyVerdictTemplate(text, rewrites.verdictSection, rewrites.verdictLine);
        const verdicts = verdictLines(text);
        if (verdicts.length !== 1) violations.push(`${squad.id}/${role} (${role0.kind}) must have exactly one VERDICT line, found ${verdicts.length}`);
      }
      guard(text, `${squad.id}/${role}`);
      const tools = [...new Set(rendered.flatMap((item) => item.tools))];
      const description = rendered[0].description;
      roleEntries.push({ key: role, kind: role0.kind, name: display, header, hasBody: true });
      agents.push({
        name: role,
        upstream: rendered.map((item) => item.source.upstream),
        description,
        role: ROLE_KIND_CLASS[role0.kind],
        kind: role0.kind,
        tools,
        instructions: text.trim(),
      });
    }

    const squadData = {
      id: squad.id,
      pack: squad.id,
      templateId: squad.templateId,
      name: squad.name,
      builtinSkills: core.map((key) => bundles.get(key)),
      leaderSkills: squad.leaderSkills.map((key) => bundleName(key.replace(/^kv-/, ""))),
      roleSkills: Object.fromEntries(
        Object.entries(squad.roleSkills ?? {}).map(([role, keys]) => [role, keys.map((key) => bundles.get(key))]),
      ),
      promoted: (squad.promoted ?? []).map((key) => bundles.get(key)),
      roles: roleEntries,
    };
    packs.set(squad.id, { skills, agents, squad: squadData });
  }

  for (const name of readdirSync(patchesDir)) {
    if (!name.endsWith(".json")) continue;
    if (!usedPatchFiles.has(name.replace(/\.json$/, ""))) {
      violations.push(`kitviet-patches/${name} matches no skill or agent`);
    }
  }
  for (const rule of rewrites.rules) {
    if (!ruleHits.has(rule.name)) violations.push(`rewrite rule ${rule.name} matched nothing (stale rule)`);
  }
  // Bundle names are unique across the packs, so the quickstart's name lookup is unambiguous.
  const seen = new Map();
  for (const [pack, entry] of packs) {
    for (const skill of entry.skills) {
      if (seen.has(skill.name)) violations.push(`skill name ${skill.name} is in ${seen.get(skill.name)} and ${pack}`);
      seen.set(skill.name, pack);
    }
  }
  if (violations.length > 0) {
    throw new Error(`${violations.length} violations:\n${violations.slice(0, 120).join("\n")}`);
  }
  return packs;
}

// Every pack module exports the same names, so kitviet-packs.ts can load any of them. A pack file holds
// the heavy text only (skills and agent bodies); it is imported on demand. The names and roles of every
// squad are in squads.ts, which the template registry imports.
export function renderPack(pack, entry) {
  const skills = JSON.stringify(entry.skills, null, 2);
  const agents = JSON.stringify(entry.agents, null, 2);
  return `${HEADER}
import type { KitvietAgent, KitvietSkill } from "./types";

/** Skills of the ${pack.id} pack. The quickstart loads this file on demand. */
export const KITVIET_BUILTIN_SKILLS: readonly KitvietSkill[] = ${skills};

/** Agents of the ${pack.id} pack, by role key, with the body each role runs on (none for a lead). */
export const KITVIET_AGENTS: readonly KitvietAgent[] = ${agents};
`;
}

// The names-only module: every squad's skill names and roles, with no skill text and no
// agent body. It is small enough for the template registry to import statically.
export function renderSquads(packs) {
  const squads = SQUADS.map((squad) => packs.get(squad.id).squad);
  return `${HEADER}
import type { KitvietSquad } from "./types";

/** The Kit Viet squads of the pack, in the order of kitviet-squads.json. */
export const KITVIET_SQUADS: readonly KitvietSquad[] = ${JSON.stringify(squads, null, 2)};
`;
}

const HEADER = `// Generated by scripts/generate-kitviet-pack.mjs from packages/core/squads/kitviet-upstream,
// kitviet-squads.json and kitviet-skills. Do not edit by hand; run \`pnpm generate:kitviet\`.`;

const TYPES = `${HEADER}

export type { BuiltinSkill, BuiltinSkillFile } from "../sdlc-skills-data";
import type { BuiltinSkill } from "../sdlc-skills-data";
import type { KvRoleKind } from "../kitviet-agent-instructions";

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

/** The loop role an agent plays (spec 5.1). A lead is not an upstream agent: it has no body. */
export type KitvietRole = "lead" | "collector" | "drafter" | "gate" | "handover";

export interface KitvietAgent {
  /** The role key of the squad, e.g. \`legal_reader\`. */
  name: string;
  /** The upstream agent names this role runs on (two for a merged role). */
  upstream: readonly string[];
  description: string;
  role: KitvietRole;
  kind: KvRoleKind;
  tools: readonly string[];
  /** The role body without its header; the template adds the kind text and the header. */
  instructions: string;
}

/** One role of a squad, names only. The agent body of a role is in its pack file, loaded on demand. */
export interface KitvietSquadRole {
  /** The role key, e.g. \`legal_reader\`. */
  key: string;
  kind: KvRoleKind;
  /** The display name the agent is created with, e.g. \`Legal Reader\`. */
  name: string;
  /** "You are the <name> in a <squad> squad, unattended." The only opener of the agent. */
  header: string;
  /** Whether the role has a body in its pack file. A lead does not. */
  hasBody: boolean;
}

export interface KitvietSquad {
  id: string;
  /** The pack file that holds this squad's skills and agent bodies (its id). */
  pack: string;
  templateId: string;
  name: string;
  /** The core skills every member gets: shared, authored and this trade's core. */
  builtinSkills: readonly string[];
  leaderSkills: readonly string[];
  roleSkills: Readonly<Record<string, readonly string[]>>;
  promoted: readonly string[];
  roles: readonly KitvietSquadRole[];
}
`;

// Every generated file as { path, content }, keyed by its file name in kitviet-skills-data.
export function generateOutputs() {
  const packs = buildPacks();
  const outputs = new Map([["types.ts", TYPES]]);
  for (const pack of PACKS) {
    const entry = packs.get(pack.id);
    outputs.set(`${pack.file}.ts`, renderPack(pack, entry));
  }
  outputs.set("squads.ts", renderSquads(packs));
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
