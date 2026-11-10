#!/usr/bin/env node
// Generates packages/core/squads/kitviet-skills-data/*.ts, the Kit Viet skill, agent and squad data
// the squad quickstart imports, from the vendored upstream in packages/core/squads/kitviet-upstream.
// Inputs:
//   kitviet-rewrites.json        the rewrite table (Pro blocks, platform sections, file names, verdicts)
//   kitviet-patches/<name>.json  exact-match patches per skill or agent (upstream text)
//   kitviet-squads/base.json     the shared skills and phrases every squad loads
//   kitviet-squads/<id>.json     one squad: trade folder, agent -> role key and kind, core skills,
//                                role skills, deferred skills, leader skills (spec 5.10, 6, F17)
//   kitviet-extensions/<id>.json one extension pack: skills attached to existing squads (spec 7)
//   kitviet-skills/<kv-*>/       authored skills (spec 5.5), shared by every squad
// A new squad or extension is one config file; nothing in this script names a trade or a squad.
// Run `pnpm generate:kitviet`; `--check` regenerates in memory and fails when the committed output is
// stale. The upstream is never read from the network.
//
// Each squad has its own pack file (admin.ts, ...): the trade skills it lists and its agents rendered
// with that squad's role keys. An extension has a pack file too (support.ts, ...): its listed skills
// plus their kv: dependency closure, rendered with neutral agent wording (it has no roles). squads.ts
// and extensions.ts hold names only; packs.ts maps each pack id to its loader. The shared skills
// (common.ts) are rendered once, with neutral wording for agent names, because every squad loads them.
//
// Ownership (one owner per skill): a skill is rendered by one pack only. A squad lists its skills in
// builtinSkills, roleSkills or deferredSkills; an extension lists its own in skills and the generator
// adds the skills they reference. Listing one skill in two configs, or a closure member a squad renders,
// fails the build.
//
// Reference rules (F17), applied per pack to every skill and agent body:
//   kv:<skill> in the pack's resolution set (core + roleSkills; extension: its whole set) -> bundle kv--<skill>
//   kv:<skill> elsewhere                                                       -> "nhờ lead tạo yêu cầu"
//   <agent> mapped to a role of the squad                                      -> the role key
//   <agent> not in the squad                                                   -> "nhờ lead"
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
const rewrites = readJson(join(squadsDir, "kitviet-rewrites.json"));
const upstream = readJson(join(upstreamDir, "UPSTREAM.json"));

// The configs of one run: the shared base, one entry per squad (sorted by `order`, then id) and one per
// extension (sorted by id). The file name without .json is the id, which is also the pack id.
function readConfigFolder(dir) {
  if (!existsSync(dir)) throw new Error(`${dir} is missing`);
  return readdirSync(dir)
    .filter((file) => file.endsWith(".json"))
    .sort()
    .map((file) => ({ id: file.replace(/\.json$/, ""), ...readJson(join(dir, file)) }));
}

const byId = (a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);

export function loadConfig(root) {
  const base = readJson(join(root, "kitviet-squads", "base.json"));
  const squads = readConfigFolder(join(root, "kitviet-squads"))
    .filter((squad) => squad.id !== "base")
    .sort((a, b) => (a.order ?? 1000) - (b.order ?? 1000) || byId(a, b));
  const extensions = readConfigFolder(join(root, "kitviet-extensions")).sort(byId);
  return { shared: base.sharedBuiltinSkills, sharedAgentPhrases: base.sharedAgentPhrases ?? {}, squads, extensions };
}

// The configs of the current run. generateOutputs sets it from its configRoot.
let kvConfig = loadConfig(squadsDir);

// Every pack the run writes, in order: common, then the squads, then the extensions. A pack's file
// name is its id.
export function packList() {
  return [
    { id: "common", file: "common" },
    ...kvConfig.squads.map((squad) => ({ id: squad.id, file: squad.id })),
    ...kvConfig.extensions.map((extension) => ({ id: extension.id, file: extension.id })),
  ];
}


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

// Words of a role key that are written as acronyms, not capitalised: `b2b_lead` -> `B2B Lead`, `pr_lead` -> `PR Lead`.
const ACRONYMS = new Map([
  ["b2b", "B2B"],
  ["pr", "PR"],
]);

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

// Loads the upstream folders once: skill folders by key, agent files by name, per trade and per shared
// folder. Every trade a config names is loaded; a trade with no folder fails the run.
function configTrades() {
  return [...new Set([...kvConfig.squads.map((squad) => squad.trade), ...kvConfig.extensions.map((extension) => extension.trade)])];
}

function loadUpstream() {
  const dropped = new Set(rewrites.droppedSkills);
  const commonSkills = new Map();
  for (const folder of listFolders(join(upstreamDir, "common", "skills"))) {
    if (dropped.has(folder)) continue;
    commonSkills.set(folder, { dir: join(upstreamDir, "common", "skills", folder) });
  }
  const tradeSkills = new Map();
  const tradeAgents = new Map();
  for (const trade of configTrades()) {
    const dir = join(upstreamDir, trade);
    if (!existsSync(dir)) throw new Error(`kitviet-upstream/${trade} is missing; vendor it and list it in NOTICE.md`);
    const skills = new Map();
    for (const folder of listFolders(join(dir, "skills"))) {
      if (dropped.has(folder)) continue;
      skills.set(folder, { dir: join(dir, "skills", folder) });
    }
    tradeSkills.set(trade, skills);
    const agents = new Map();
    for (const name of listAgentNames(join(dir, "agents"))) agents.set(name, join(dir, "agents", `${name}.md`));
    tradeAgents.set(trade, agents);
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

// The shared skill keys: base skills and the authored core skill, in the base config's order.
function sharedKeys() {
  return kvConfig.shared;
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

// The prose a skill's references are read from: SKILL.md and its markdown reference files, with the Pro
// block and the platform sections removed as transformBody removes them.
function skillProse(dir) {
  const files = ["SKILL.md", ...skillFiles(dir).filter((file) => file.endsWith(".md"))];
  return files
    .map((file) => {
      let text = readFileSync(join(dir, file), "utf8").replace(/\r\n/g, "\n");
      for (const block of rewrites.removeBlocks) text = removeBlocks(text, block);
      for (const title of rewrites.removeSections) text = removeSection(text, title);
      return text;
    })
    .join("\n");
}

// The skills an extension renders (spec 7): its listed skills, then every trade or shared skill their
// prose names with kv:<key>, transitively. An agent name wins over a skill of the same name, as in
// resolveReferences. A shared skill is attached by name and rendered once, in common. A trade skill that
// another config renders (`elsewhere(key)`) is not pulled in: it is returned in `cut`, and the reference
// reads "nhờ lead tạo yêu cầu" (F17), the same as a reference outside any pack.
function extensionMembers(extension, up, elsewhere) {
  const trade = up.tradeSkills.get(extension.trade);
  const agents = allAgentNames(up);
  const members = [...new Set(extension.skills)];
  const cut = [];
  for (let at = 0; at < members.length; at += 1) {
    const entry = trade.get(members[at]);
    if (!entry) continue;
    for (const match of skillProse(entry.dir).matchAll(/kv:([a-z0-9-]+)/g)) {
      const ref = match[1];
      if (ref === "pro" || agents.has(ref)) continue;
      if (!trade.has(ref) && !up.commonSkills.has(ref) && !up.authored.has(ref)) continue;
      if (members.includes(ref) || cut.includes(ref)) continue;
      if (trade.has(ref) && elsewhere(ref)) cut.push(ref);
      else members.push(ref);
    }
  }
  return { members, cut };
}

// The bundle name of a skill key: a shared key keeps its name (kv--<key>), a trade key takes its rename.
function bundleFor(key, trade, up) {
  if (up.commonSkills.has(key)) return bundleName(key);
  if (up.authored.has(key)) return bundleName(key.replace(/^kv-/, ""));
  return bundleName((rewrites.renamedSkills[trade] ?? {})[key] ?? key);
}

const FLOWS = ["pipeline", "authored"];

// Ownership and rendering (spec 5.5, 5.10, 7). One config owns each (trade, skill): a squad lists its
// skills in builtinSkills, roleSkills and deferredSkills; an extension lists its own in skills, and
// its closure is added. A (trade, skill) listed twice, or a closure member a squad owns, fails the run.
// Every bundle name is rendered by one pack (checked below, as before).
function buildPacks() {
  violations = [];
  ruleHits = new Map();
  for (const file of ["LICENSE", "NOTICE.md", "UPSTREAM.json"]) {
    if (!existsSync(join(upstreamDir, file))) throw new Error(`kitviet-upstream/${file} is missing`);
  }
  const sourceConfig = { source: "kitviet", upstream: upstream.repo, ref: upstream.ref, license: upstream.license };
  const authoredConfig = { source: "kitviet", upstream: "authored in this repository", ref: "authored", license: "repository" };
  const up = loadUpstream();
  const knownSkills = allSkillKeys(up);
  const knownAgents = allAgentNames(up);
  const usedPatchFiles = new Set();
  const packs = new Map();
  const squads = kvConfig.squads;
  const extensions = kvConfig.extensions;

  // Pack ids are unique across squads and extensions. A target (a SquadTemplate id or a company squad
  // key) belongs to one extension only.
  const packIds = [...squads, ...extensions].map((entry) => entry.id);
  for (const id of new Set(packIds)) {
    if (packIds.filter((other) => other === id).length > 1) violations.push(`pack id ${id} is used by two configs`);
  }
  if (packIds.includes("common")) violations.push("a squad or extension may not be named common");
  const targetOwner = new Map();
  for (const extension of extensions) {
    if (!Array.isArray(extension.targets) || extension.targets.length === 0) {
      violations.push(`${extension.id}: targets must name at least one squad`);
    }
    for (const target of extension.targets ?? []) {
      if (targetOwner.has(target)) violations.push(`${extension.id}: target ${target} is also in ${targetOwner.get(target)}`);
      targetOwner.set(target, extension.id);
    }
  }

  // The shared pack: base skills and authored skills, in neutral wording. Every authored skill must be
  // shared or a leader skill of some squad, and every shared key must exist.
  const sharedBundles = new Map([
    ...[...up.commonSkills.keys()].map((key) => [key, bundleName(key)]),
    ...[...up.authored.keys()].map((key) => [key, bundleName(key.replace(/^kv-/, ""))]),
  ]);
  const leaderKeys = new Set(squads.flatMap((squad) => squad.leaderSkills));
  for (const key of up.commonSkills.keys()) {
    if (!sharedKeys().includes(key)) violations.push(`common skill ${key} is not in sharedBuiltinSkills (orphan)`);
  }
  for (const key of up.authored.keys()) {
    if (!sharedKeys().includes(key) && !leaderKeys.has(key)) violations.push(`authored skill ${key} is used by no squad`);
  }
  for (const key of sharedKeys()) {
    if (!sharedBundles.has(key)) violations.push(`kitviet-squads/base.json names shared skill ${key}, which does not exist`);
  }
  const sharedCtx = buildContext({
    mode: "neutral",
    skills: new Map([...sharedBundles].filter(([key]) => sharedKeys().includes(key))),
    knownSkills,
    roles: new Map(),
    knownAgents,
    agentPhrases: kvConfig.sharedAgentPhrases,
  });
  const commonSkills = [];
  for (const key of [...up.commonSkills.keys()].sort()) {
    const entry = up.commonSkills.get(key);
    usedPatchFiles.add(key);
    commonSkills.push(
      renderSkill({ dir: entry.dir, key, bundle: bundleName(key), ctx: sharedCtx, config: sourceConfig, label: `skill ${key}`, files: skillFiles(entry.dir).sort() }),
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

  // Claims: which config lists each (trade, skill). A second claim by another config is a violation.
  const claims = new Map();
  const claim = (trade, key, owner) => {
    const id = `${trade}/${key}`;
    if (claims.has(id) && claims.get(id) !== owner) violations.push(`${key} (${trade}) is listed by ${claims.get(id)} and by ${owner}`);
    else claims.set(id, owner);
  };

  // Squads: the checks that were per squad, and what each one lists.
  const plans = squads.map((squad) => {
    const owner = `squad ${squad.id}`;
    const trade = up.tradeSkills.get(squad.trade);
    const bundles = squadSkillBundles(up, squad);
    const { roles, roleOf } = squadRoles(squad, up);
    const core = [...sharedKeys(), ...squad.builtinSkills];
    const roleSkillKeys = Object.values(squad.roleSkills ?? {}).flat();
    const resolution = new Set([...core, ...roleSkillKeys]);
    for (const key of resolution) {
      if (!bundles.has(key)) violations.push(`${squad.id}: ${key} is in neither the shared pack nor the ${squad.trade} trade`);
    }
    for (const key of squad.builtinSkills) {
      if (!trade.has(key)) violations.push(`${squad.id}: builtinSkills names ${key}, which is not in ${squad.trade}`);
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

    // flow "pipeline" (default): the squad maps upstream agents to roles beyond the lead. flow "authored":
    // the trade has no pipeline for this squad; its lead flow is written in the template, and it may not
    // attach an upstream pipeline skill (day-chuyen-*), whose text describes another squad's stages.
    const flow = squad.flow ?? "pipeline";
    if (!FLOWS.includes(flow)) violations.push(`${squad.id}: flow ${flow} is not one of ${FLOWS.join(", ")}`);
    if (flow === "pipeline" && [...roles.values()].every((role) => role.kind === "kv_lead")) {
      violations.push(`${squad.id}: a pipeline squad needs an upstream role besides the lead (or set "flow": "authored")`);
    }

    const deferred = new Set(squad.deferredSkills ?? []);
    for (const key of deferred) {
      if (!trade.has(key)) violations.push(`${squad.id}: deferredSkills names ${key}, which is not in ${squad.trade}`);
      if (resolution.has(key)) violations.push(`${squad.id}: ${key} is deferred and also attached`);
    }
    const listed = [...new Set([...squad.builtinSkills, ...roleSkillKeys, ...deferred])];
    for (const key of listed) {
      if (flow === "authored" && key.startsWith("day-chuyen-")) {
        violations.push(`${squad.id}: an authored flow cannot attach the upstream pipeline ${key}`);
      }
      claim(squad.trade, key, owner);
    }
    return { squad, trade, bundles, roles, roleOf, resolution, listed };
  });

  // Extensions: the skills each one lists, then its closure. A closure member a squad lists is a
  // violation (list it in the extension and remove it from the squad, or drop the reference).
  const extensionPlans = extensions.map((extension) => {
    const owner = `extension ${extension.id}`;
    const trade = up.tradeSkills.get(extension.trade);
    for (const key of extension.skills) {
      if (!trade.has(key)) violations.push(`${extension.id}: skills names ${key}, which is not in ${extension.trade}`);
      claim(extension.trade, key, owner);
    }
    return { extension, trade, owner, members: [] };
  });
  for (const plan of extensionPlans) {
    const { extension, owner } = plan;
    const elsewhere = (key) => {
      const holder = claims.get(`${extension.trade}/${key}`);
      return holder !== undefined && holder !== owner;
    };
    const { members, cut } = extensionMembers(extension, up, elsewhere);
    plan.members = members;
    plan.cut = cut;
    for (const key of members) {
      if (up.commonSkills.has(key) || up.authored.has(key) || extension.skills.includes(key)) continue;
      claim(extension.trade, key, owner);
    }
  }

  // Squad packs: the listed skills, rendered in the squad's wording, then its agents.
  for (const plan of plans) {
    const { squad, trade, bundles, roles, roleOf, resolution, listed } = plan;
    const squadCtx = buildContext({
      mode: "squad",
      skills: new Map([...resolution].map((key) => [key, bundles.get(key)])),
      knownSkills,
      roles: roleOf,
      knownAgents,
      agentPhrases: {},
    });
    const skills = [];
    for (const key of [...listed].sort()) {
      const entry = trade.get(key);
      if (!entry) continue;
      usedPatchFiles.add(key);
      skills.push(
        renderSkill({
          dir: entry.dir,
          key,
          bundle: bundles.get(key),
          ctx: squadCtx,
          config: sourceConfig,
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
      builtinSkills: [...sharedKeys(), ...squad.builtinSkills].map((key) => bundles.get(key)),
      leaderSkills: squad.leaderSkills.map((key) => bundleName(key.replace(/^kv-/, ""))),
      roleSkills: Object.fromEntries(
        Object.entries(squad.roleSkills ?? {}).map(([role, keys]) => [role, keys.map((key) => bundles.get(key))]),
      ),
      promoted: (squad.promoted ?? []).map((key) => bundles.get(key)),
      roles: roleEntries,
    };
    packs.set(squad.id, { skills, agents, squad: squadData });
  }

  // Extension packs: the listed skills and their closure, in neutral wording (an extension has no roles),
  // and the bundle names each target attaches (its shared skills included, by name).
  for (const plan of extensionPlans) {
    const { extension, trade, members } = plan;
    const memberBundles = new Map(members.map((key) => [key, bundleFor(key, extension.trade, up)]));
    const extensionCtx = buildContext({
      mode: "squad",
      skills: memberBundles,
      knownSkills,
      roles: new Map(),
      knownAgents,
      agentPhrases: {},
    });
    const skills = [];
    for (const key of members.filter((key) => trade.has(key) && !up.commonSkills.has(key) && !up.authored.has(key)).sort()) {
      const entry = trade.get(key);
      usedPatchFiles.add(key);
      skills.push(
        renderSkill({
          dir: entry.dir,
          key,
          bundle: memberBundles.get(key),
          ctx: extensionCtx,
          config: sourceConfig,
          label: `skill ${extension.trade}/${key}`,
          files: skillFiles(entry.dir).sort(),
        }),
      );
    }
    packs.set(extension.id, {
      skills,
      agents: [],
      squad: null,
      extension: {
        id: extension.id,
        name: extension.name,
        pack: extension.id,
        targets: [...extension.targets],
        builtinSkills: members.map((key) => memberBundles.get(key)),
        cut: plan.cut.map((key) => bundleFor(key, extension.trade, up)),
      },
    });
  }

  // Orphan guard (spec 5.10, K8): every skill of a squad's trade is rendered by some pack (attached,
  // deferred or extension-owned) or shared under another trade's bundle name. A skill in none is a leftover.
  const renderedNames = new Set([...packs.values()].flatMap((entry) => entry.skills.map((skill) => skill.name)));
  for (const plan of plans) {
    for (const key of plan.trade.keys()) {
      if (!renderedNames.has(bundleFor(key, plan.squad.trade, up))) {
        violations.push(`${plan.squad.id}: ${key} is attached to nothing and not deferred (orphan)`);
      }
    }
  }

  for (const name of readdirSync(patchesDir)) {
    if (!name.endsWith(".json")) continue;
    if (!usedPatchFiles.has(name.replace(/\.json$/, ""))) {
      violations.push(`kitviet-patches/${name} matches no skill or agent`);
    }
  }
  for (const rule of rewrites.rules) {
    // An optional rule (its text is also caught by the forbidden-text guard) may match nothing in this run.
    if (!ruleHits.has(rule.name) && !rule.optional) violations.push(`rewrite rule ${rule.name} matched nothing (stale rule)`);
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
  const squads = kvConfig.squads.map((squad) => packs.get(squad.id).squad);
  return `${HEADER}
import type { KitvietSquad } from "./types";

/** The Kit Viet squads of the pack, in the order of kitviet-squads/ (by order, then id). */
export const KITVIET_SQUADS: readonly KitvietSquad[] = ${JSON.stringify(squads, null, 2)};
`;
}

// The names-only module of the extension packs: which existing squads each one attaches to, and the
// bundle names it attaches. No skill text; the extension's pack file holds the text.
export function renderExtensions(packs) {
  const extensions = kvConfig.extensions.map((extension) => packs.get(extension.id).extension);
  return `${HEADER}
import type { KitvietExtension } from "./types";

/** The Kit Viet extension packs (spec 7), in the order of kitviet-extensions/. */
export const KITVIET_EXTENSIONS: readonly KitvietExtension[] = ${JSON.stringify(extensions, null, 2)};
`;
}

// The loader of every pack by its id. kitviet-packs.ts imports a pack only through its loader, so a
// pack no squad or extension uses is never downloaded.
export function renderLoaders() {
  const lines = packList().map((pack) => `  ${JSON.stringify(pack.id)}: () => import(${JSON.stringify(`./${pack.file}`)}),`);
  return `${HEADER}

/** One loader per pack id: common, then each squad and extension. */
export const KITVIET_PACK_LOADERS = {
${lines.join("\n")}
} as const;
`;
}

const HEADER = `// Generated by scripts/generate-kitviet-pack.mjs from packages/core/squads/kitviet-upstream,
// kitviet-squads/, kitviet-extensions/ and kitviet-skills. Do not edit by hand; run \`pnpm generate:kitviet\`.`;

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

/** One extension pack (spec 7): the Kit Viet skills attached to the existing squads it targets. */
export interface KitvietExtension {
  id: string;
  name: string;
  /** The pack file that holds its skills (its id). */
  pack: string;
  /** The existing squads it attaches to: a SquadTemplate id or a company squad key. */
  targets: readonly string[];
  /** The bundle names attached to each target: the listed skills, their closure, and shared skills. */
  builtinSkills: readonly string[];
  /** Referenced skills another squad or extension renders; they stay out, and their references read "nhờ lead". */
  cut: readonly string[];
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

// Every generated file as { path, content }, keyed by its file name in kitviet-skills-data. configRoot is
// a folder holding kitviet-squads/ and kitviet-extensions/ (the tests point it at a copy); it defaults to
// the repository's configs.
export function generateOutputs({ configRoot = squadsDir } = {}) {
  kvConfig = loadConfig(configRoot);
  const packs = buildPacks();
  const outputs = new Map([["types.ts", TYPES]]);
  for (const pack of packList()) outputs.set(`${pack.file}.ts`, renderPack(pack, packs.get(pack.id)));
  outputs.set("squads.ts", renderSquads(packs));
  outputs.set("extensions.ts", renderExtensions(packs));
  outputs.set("packs.ts", renderLoaders());
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
