#!/usr/bin/env node
// Updates the vendored Kit Viet upstream to its latest release (or a given ref) and regenerates the pack.
//
//   pnpm kitviet:update              latest release tag, or main HEAD when there is no tag
//   pnpm kitviet:update <ref>        a tag, a branch, or a full commit SHA
//   pnpm kitviet:update -- --dry-run  run every step in a temporary copy, write nothing
//
// Env: KITVIET_REMOTE overrides the remote (a URL or a local path; tests and offline runs use a local clone).
//
// Steps: resolve the ref (git ls-remote, read only) -> stop when it equals UPSTREAM.json -> fetch the commit
// (git fetch, read only) into a temporary folder -> stage a copy of the repo's squad pack and scripts -> in the
// stage: capture the replaced upgrade texts, vendor the trades, regenerate, run --check, the generator and
// squad tests, and tsc -> replace the real kitviet-upstream, kitviet-skills-data and (when texts were captured)
// kitviet-previous-texts.ts, with a backup that restores them if a copy fails. Nothing is committed or pushed.
//
// The upstream is data: its scripts are never run. Only the files under <trade>/claude-code/kv are copied.

import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..");
const squadsRel = "packages/core/squads";
const DEFAULT_REMOTE = "https://github.com/mochistudio-vn/kitviet";

// The paths the update replaces. A change to any of them that is not committed stops the update.
const WRITE_PATHS = [`${squadsRel}/kitviet-upstream`, `${squadsRel}/kitviet-skills-data`, `${squadsRel}/kitviet-previous-texts.ts`];

// Every input the registry capture reads: the composition of the Kit Viet texts (the role bodies, the squad templates,
// the shared blocks) and the pack configs and sources the generator reads. The capture takes the "before" texts from
// the working tree, so an uncommitted change here would be captured as if it had shipped. Other squad files are not
// listed: they do not feed a Kit Viet text.
export const GUARDED_PATHS = [
  ...WRITE_PATHS,
  `${squadsRel}/kitviet-rewrites.json`,
  `${squadsRel}/kitviet-squads`,
  `${squadsRel}/kitviet-patches`,
  `${squadsRel}/kitviet-skills`,
  `${squadsRel}/kitviet-templates`,
  `${squadsRel}/kitviet-agent-instructions.ts`,
  `${squadsRel}/kitviet-squad-roles.ts`,
  `${squadsRel}/templates.ts`,
  "packages/core/agents/instruction-blocks.ts",
  "scripts/generate-kitviet-pack.mjs",
  "scripts/lib/skill-bundle.mjs",
];

// ---------------------------------------------------------------------------------------------------------
// Pure functions (unit tested in kitviet-update.test.mjs)
// ---------------------------------------------------------------------------------------------------------

const RELEASE_TAG = /^v\d+\.\d+\.\d+$/;
const FULL_SHA = /^[0-9a-f]{40}$/;

/** -1, 0 or 1: the order of two release tags such as "v1.10.0" and "v1.9.0". */
export function compareVersions(a, b) {
  const pa = a.slice(1).split(".").map(Number);
  const pb = b.slice(1).split(".").map(Number);
  for (let i = 0; i < 3; i += 1) {
    if (pa[i] !== pb[i]) return pa[i] > pb[i] ? 1 : -1;
  }
  return 0;
}

/** The highest plain release tag (vX.Y.Z); pre-releases and other names are not releases. */
export function latestReleaseTag(names) {
  const tags = names.filter((name) => RELEASE_TAG.test(name));
  if (tags.length === 0) return null;
  return tags.reduce((best, tag) => (compareVersions(tag, best) > 0 ? tag : best));
}

/**
 * The output of `git ls-remote` as a Map from ref name to commit SHA. For an annotated tag the peeled line
 * (`refs/tags/x^{}`) gives the commit, so it replaces the tag object's SHA.
 */
export function parseRemoteRefs(output) {
  const refs = new Map();
  const peeled = new Map();
  for (const line of output.split("\n")) {
    const [sha, name] = line.trim().split(/\s+/);
    if (!sha || !name) continue;
    if (name.endsWith("^{}")) peeled.set(name.slice(0, -3), sha);
    else refs.set(name, sha);
  }
  for (const [name, sha] of peeled) refs.set(name, sha);
  return refs;
}

/**
 * The commit to update to. Without a request: the latest release tag, else HEAD. A request is a full SHA, a
 * tag, or a branch. `version` is the tag name, or null for a commit or a branch.
 */
export function pickTarget({ requested, refs }) {
  if (requested !== undefined) {
    if (FULL_SHA.test(requested)) return { sha: requested, version: null };
    if (refs.has(`refs/tags/${requested}`)) return { sha: refs.get(`refs/tags/${requested}`), version: requested };
    if (refs.has(`refs/heads/${requested}`)) return { sha: refs.get(`refs/heads/${requested}`), version: null };
    throw new Error(`unknown ref ${requested} on the remote`);
  }
  const tag = latestReleaseTag([...refs.keys()].filter((name) => name.startsWith("refs/tags/")).map((name) => name.slice("refs/tags/".length)));
  if (tag) return { sha: refs.get(`refs/tags/${tag}`), version: tag };
  if (!refs.has("HEAD")) throw new Error("the remote has no release tag and no HEAD");
  return { sha: refs.get("HEAD"), version: null };
}

/** The `git status --porcelain` lines whose path is one of the write paths or inside one. */
export function dirtyPaths(porcelain, writePaths) {
  return porcelain
    .split("\n")
    .filter((line) => line.trim() !== "")
    .filter((line) => {
      const path = line.slice(3).split(" -> ").pop();
      return writePaths.some((write) => path === write || path.startsWith(`${write}/`));
    });
}

/** The license a LICENSE file states. Only MIT is accepted; anything else needs a human decision first. */
export function detectLicense(text) {
  if (/^MIT License/m.test(text)) return "MIT";
  throw new Error("unknown license in the upstream LICENSE; review it and extend detectLicense before vendoring");
}

function walk(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0))
    .flatMap((entry) => {
      const path = join(dir, entry.name);
      return entry.isDirectory() ? walk(path) : [path];
    });
}

// Text is stored with LF endings, as the vendored copy always has been. Binary files are copied as they are.
function vendoredBytes(path) {
  const raw = readFileSync(path);
  if (raw.includes(0)) return raw;
  const text = raw.toString("utf8");
  return text.includes("\r\n") ? Buffer.from(text.replace(/\r\n/g, "\n"), "utf8") : raw;
}

/**
 * Copies the skills and agents of each trade (<trade>/claude-code/kv) into outDir. A file byte-identical in
 * every trade goes once to common/; the other files stay under their trade. Returns the counts and the
 * paths whose written bytes differ from the upstream file (empty when the copy is exact).
 */
export function vendorUpstream({ treeDir, trades, outDir }) {
  const sources = new Map();
  for (const trade of trades) {
    const root = join(treeDir, trade, "claude-code", "kv");
    if (!existsSync(root)) throw new Error(`the upstream has no ${trade}/claude-code/kv folder`);
    const files = new Map();
    for (const sub of ["skills", "agents"]) {
      for (const file of walk(join(root, sub))) {
        files.set(`${sub}/${relative(join(root, sub), file).split(sep).join("/")}`, vendoredBytes(file));
      }
    }
    sources.set(trade, files);
  }
  const allRels = new Set([...sources.values()].flatMap((files) => [...files.keys()]));
  const first = sources.get(trades[0]);
  const common = new Set(
    [...allRels].filter((rel) =>
      trades.every((trade) => sources.get(trade).has(rel) && sources.get(trade).get(rel).equals(first.get(rel))),
    ),
  );

  const expected = new Map();
  for (const rel of common) expected.set(join(outDir, "common", rel), first.get(rel));
  for (const trade of trades) {
    for (const [rel, bytes] of sources.get(trade)) {
      if (!common.has(rel)) expected.set(join(outDir, trade, rel), bytes);
    }
  }
  const mismatches = [];
  for (const [path, bytes] of expected) {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, bytes);
    if (!readFileSync(path).equals(bytes)) mismatches.push(path);
  }
  return { trades: trades.length, common: common.size, files: expected.size, mismatches };
}

/**
 * The upgrade texts to append to the registry: a text is captured only when the release replaces it (its
 * role or squad text changed or disappeared) and the registry does not list it yet. `before` and `after` hold
 * the current composed text per role key (agents) and per squad template id (squads).
 */
export function registryAdditions(previous, before, after) {
  const agents = {};
  for (const [key, old] of Object.entries(before.agents)) {
    if (old === after.agents[key]) continue;
    if (previous.agents[key]?.includes(old)) continue;
    agents[key] = [old];
  }
  const squads = [];
  for (const [id, old] of Object.entries(before.squads)) {
    if (old === after.squads[id]) continue;
    if (previous.squads.includes(old) || squads.includes(old)) continue;
    squads.push(old);
  }
  return { agents, squads };
}

/**
 * The registry source with the additions inserted. Existing lines are not touched: a text goes before the
 * closing line of its role's array (or the squad list), and a new role gets its own block at the end.
 */
export function appendRegistryEntries(source, { agents, squads }) {
  let out = source;
  if (squads.length > 0) {
    const start = out.indexOf("export const KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS");
    const end = out.indexOf("\n];", start);
    if (start < 0 || end < 0) throw new Error("the registry has no squad list to append to");
    out = out.slice(0, end) + squads.map((text) => `\n  ${JSON.stringify(text)},`).join("") + out.slice(end);
  }
  let newBlocks = "";
  for (const [key, texts] of Object.entries(agents)) {
    const start = out.indexOf("export const KITVIET_PREVIOUS_AGENT_INSTRUCTIONS");
    const close = out.indexOf("\n};", start);
    const lines = texts.map((text) => `\n    ${JSON.stringify(text)},`).join("");
    const keyAt = out.indexOf(`\n  ${JSON.stringify(key)}: [`, start);
    if (keyAt >= 0 && keyAt < close) {
      const end = out.indexOf("\n  ],", keyAt);
      out = out.slice(0, end) + lines + out.slice(end);
    } else {
      newBlocks += `\n  ${JSON.stringify(key)}: [${lines}\n  ],`;
    }
  }
  if (newBlocks !== "") {
    const start = out.indexOf("export const KITVIET_PREVIOUS_AGENT_INSTRUCTIONS");
    const close = out.indexOf("\n};", start);
    out = out.slice(0, close) + newBlocks + out.slice(close);
  }
  return out;
}

/** The skill folders of a vendored upstream as "<scope>/<folder>", where scope is common or a trade. */
export function upstreamSkillFolders(root, trades) {
  const folders = [];
  for (const scope of ["common", ...trades]) {
    const dir = join(root, scope, "skills");
    if (!existsSync(dir)) continue;
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.isDirectory()) folders.push(`${scope}/${entry.name}`);
    }
  }
  return folders.sort();
}

/** Keys added, removed and changed between two maps. */
export function summarizeByKey(before, after) {
  return {
    added: [...after.keys()].filter((key) => !before.has(key)),
    removed: [...before.keys()].filter((key) => !after.has(key)),
    changed: [...after.keys()].filter((key) => before.has(key) && before.get(key) !== after.get(key)),
  };
}

/**
 * Replaces each target with its source, in order. Before a target is replaced it is copied to backupRoot; if any
 * step fails, every target already replaced is restored, so the tree is as it was before the call.
 */
export function replaceTrees(plan, backupRoot) {
  mkdirSync(backupRoot, { recursive: true });
  const done = [];
  try {
    plan.forEach((item, index) => {
      if (!existsSync(item.from)) throw new Error(`missing source ${item.from}`);
      const recursive = item.kind === "dir";
      const backup = join(backupRoot, String(index));
      const existed = existsSync(item.to);
      if (existed) cpSync(item.to, backup, { recursive });
      done.push({ item, backup, existed, recursive });
      rmSync(item.to, { recursive: true, force: true });
      mkdirSync(dirname(item.to), { recursive: true });
      cpSync(item.from, item.to, { recursive });
    });
  } catch (err) {
    for (const { item, backup, existed, recursive } of done.reverse()) {
      rmSync(item.to, { recursive: true, force: true });
      if (existed) cpSync(backup, item.to, { recursive });
    }
    throw err;
  }
}

/** The UPSTREAM.json that records the pinned commit. */
export function renderUpstreamJson({ repo, ref, version, license, fetchedAt, trades }) {
  return `${JSON.stringify({ repo, ref, version, license, fetchedAt, trades }, null, 2)}\n`;
}

/** NOTICE.md for the vendored folder: where it came from, what it holds, and how it was checked. */
export function renderNotice({ repo, ref, version, license, fetchedAt, trades, vendor }) {
  return `# Kit Việt upstream (vendored)

Source: https://github.com/${repo}, licence ${license} (see \`LICENSE\` in this folder).
Pinned commit: \`${ref}\`${version ? ` (${version})` : ""}, recorded in \`UPSTREAM.json\`.
Fetched: ${fetchedAt}. Written by \`scripts/kitviet-update.mjs\`; do not edit by hand.

What is here: the \`claude-code/kv/skills\` and \`claude-code/kv/agents\` folders of the ${trades.length} trades listed in
\`UPSTREAM.json\`. Items byte-identical in all trades are stored once under \`common/\` (${vendor.common} paths); the rest
sit under their trade folder (${vendor.files} files in total). Nothing else is copied (no \`codex/\`, \`claude-web/\`,
\`brand/\`, zip archives or plugin metadata). Text files are stored with LF line endings.

Changes: none to the vendored files. Every change Multica applies lives in the generator
(\`scripts/generate-kitviet-pack.mjs\`), in \`packages/core/squads/kitviet-rewrites.json\`, in
\`packages/core/squads/kitviet-patches/\` and in the squad configs.

Verification: every vendored file equals its file at the pinned commit after LF normalisation (byte comparison of the
written file against the upstream file). Mismatches: ${vendor.mismatches.length}.
`;
}

// ---------------------------------------------------------------------------------------------------------
// Process steps (not unit tested; the dry run exercises them end to end)
// ---------------------------------------------------------------------------------------------------------

function run(cmd, args, cwd, label) {
  const result = spawnSync(cmd, args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, GIT_TERMINAL_PROMPT: "0" },
    maxBuffer: 256 * 1024 * 1024,
  });
  const output = `${result.stdout ?? ""}${result.stderr ?? ""}`;
  if (result.status !== 0) {
    throw new Error(`${label} failed (exit ${result.status ?? result.signal}):\n${output.split("\n").slice(-40).join("\n")}`);
  }
  return output;
}

const git = (args, cwd = repoRoot) => run("git", args, cwd, `git ${args[0]}`);

// The current composed texts of every Kit Viet role and squad, and the skill texts, read from one folder of pack
// sources. esbuild bundles the TypeScript, so the composition is the real one, not a copy of it.
async function snapshotPack(squadsDir, workDir) {
  const { build } = await import("esbuild");
  const path = (rel) => JSON.stringify(join(squadsDir, rel));
  const entry = `
import { KITVIET_PACK_LOADERS } from ${path("kitviet-skills-data/packs.ts")};
import { KITVIET_SQUADS } from ${path("kitviet-skills-data/squads.ts")};
import { loadKitvietRoleInstructions } from ${path("kitviet-packs.ts")};
import { SQUAD_TEMPLATES } from ${path("templates.ts")};
import { KITVIET_PREVIOUS_AGENT_INSTRUCTIONS, KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS } from ${path("kitviet-previous-texts.ts")};

export async function snapshot() {
  const agents: Record<string, string> = {};
  for (const squad of KITVIET_SQUADS) {
    for (const role of squad.roles) {
      const text = await loadKitvietRoleInstructions(role.key);
      if (text !== undefined) agents[role.key] = text;
    }
  }
  const squads: Record<string, string> = {};
  for (const squad of KITVIET_SQUADS) {
    const template = SQUAD_TEMPLATES.find((candidate) => candidate.id === squad.templateId);
    if (template) squads[squad.templateId] = template.instructions;
  }
  const skills: Record<string, string> = {};
  for (const load of Object.values(KITVIET_PACK_LOADERS)) {
    for (const skill of (await load()).KITVIET_BUILTIN_SKILLS) {
      skills[skill.name] = JSON.stringify([skill.description, skill.content, skill.files]);
    }
  }
  return {
    agents,
    squads,
    skills,
    previous: { agents: KITVIET_PREVIOUS_AGENT_INSTRUCTIONS, squads: KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS },
  };
}`;
  const result = await build({
    stdin: { contents: entry, resolveDir: squadsDir, sourcefile: "snapshot-entry.ts", loader: "ts" },
    bundle: true,
    format: "esm",
    platform: "node",
    write: false,
    logLevel: "silent",
  });
  const file = join(workDir, `snapshot-${Date.now()}-${Math.random().toString(36).slice(2)}.mjs`);
  writeFileSync(file, result.outputFiles[0].text);
  const module = await import(pathToFileURL(file).href);
  return module.snapshot();
}

function stageRepo(stage) {
  const skip = (src) => !/(^|\/)node_modules(\/|$)/.test(src);
  mkdirSync(join(stage, "packages/core"), { recursive: true });
  cpSync(join(repoRoot, "package.json"), join(stage, "package.json"));
  cpSync(join(repoRoot, "scripts"), join(stage, "scripts"), { recursive: true, filter: skip });
  cpSync(join(repoRoot, "packages/core"), join(stage, "packages/core"), { recursive: true, filter: skip });
  // Two squad tests read these server files by a path relative to packages/core (the company template data and
  // the starter recipes). Nothing the update writes lives there, so the copy only needs them.
  for (const rel of ["server/internal/service/company_template_data.json", "server/internal/salescollect/testdata"]) {
    mkdirSync(dirname(join(stage, rel)), { recursive: true });
    cpSync(join(repoRoot, rel), join(stage, rel), { recursive: true });
  }
  // Tests and tsc resolve packages through node_modules. The stage links the real folders, so no install is needed.
  symlinkSync(join(repoRoot, "node_modules"), join(stage, "node_modules"), "dir");
  const coreModules = join(repoRoot, "packages/core/node_modules");
  if (existsSync(coreModules)) symlinkSync(coreModules, join(stage, "packages/core/node_modules"), "dir");
}

function fileTextDiffers(a, b) {
  return !existsSync(a) || !existsSync(b) || !readFileSync(a).equals(readFileSync(b));
}

function patchesChanged(stage) {
  const dirA = join(repoRoot, squadsRel, "kitviet-patches");
  const dirB = join(stage, squadsRel, "kitviet-patches");
  const names = new Set([...readdirSync(dirA), ...readdirSync(dirB)]);
  return [...names].filter((name) => fileTextDiffers(join(dirA, name), join(dirB, name)));
}

export async function main(argv = process.argv.slice(2)) {
  const dryRun = argv.includes("--dry-run");
  const requested = argv.find((arg) => !arg.startsWith("--"));
  const remote = process.env.KITVIET_REMOTE || DEFAULT_REMOTE;
  const remoteUrl = existsSync(remote) ? pathToFileURL(resolve(remote)).href : remote;
  const squadsDir = join(repoRoot, squadsRel);
  const current = JSON.parse(readFileSync(join(squadsDir, "kitviet-upstream/UPSTREAM.json"), "utf8"));

  const target = pickTarget({ requested, refs: parseRemoteRefs(git(["ls-remote", remoteUrl])) });
  const label = (sha, version) => `${sha.slice(0, 8)}${version ? ` (${version})` : ""}`;
  if (target.sha === current.ref) {
    console.log(`Kit Viet upstream is already at ${label(current.ref, current.version)}; nothing to update.`);
    return { changed: false };
  }

  const dirty = dirtyPaths(git(["status", "--porcelain", "--", ...GUARDED_PATHS]), GUARDED_PATHS);
  if (dirty.length > 0 && !dryRun) {
    throw new Error(`uncommitted changes in the Kit Viet pack inputs or outputs; commit or stash them first:\n${dirty.join("\n")}`);
  }

  const tree = mkdtempSync(join(tmpdir(), "kitviet-upstream-"));
  const stage = mkdtempSync(join(tmpdir(), "kitviet-stage-"));
  let ok = false;
  try {
    git(["init", "-q"], tree);
    git(["fetch", "-q", "--depth", "1", remoteUrl, target.sha], tree);
    git(["checkout", "-q", "--detach", "FETCH_HEAD"], tree);
    if (git(["rev-parse", "HEAD"], tree).trim() !== target.sha) throw new Error(`fetched commit is not ${target.sha}`);
    const license = detectLicense(readFileSync(join(tree, "LICENSE"), "utf8"));
    const fetchedAt = new Date().toISOString().slice(0, 10);

    stageRepo(stage);
    const stageSquads = join(stage, squadsRel);
    const stageUpstream = join(stageSquads, "kitviet-upstream");
    const stageData = join(stageSquads, "kitviet-skills-data");
    const stageRegistry = join(stageSquads, "kitviet-previous-texts.ts");

    // The texts shipped now, before the generator changes them.
    const before = await snapshotPack(stageSquads, stage);
    const foldersBefore = upstreamSkillFolders(stageUpstream, current.trades);

    rmSync(stageUpstream, { recursive: true, force: true });
    mkdirSync(stageUpstream, { recursive: true });
    const vendor = vendorUpstream({ treeDir: tree, trades: current.trades, outDir: stageUpstream });
    if (vendor.mismatches.length > 0) throw new Error(`vendored files differ from upstream: ${vendor.mismatches.join(", ")}`);
    const foldersAfter = upstreamSkillFolders(stageUpstream, current.trades);
    const newFolders = foldersAfter.filter((folder) => !foldersBefore.includes(folder));
    const goneFolders = foldersBefore.filter((folder) => !foldersAfter.includes(folder));
    copyFileSync(join(tree, "LICENSE"), join(stageUpstream, "LICENSE"));
    const meta = { repo: current.repo, ref: target.sha, version: target.version, license, fetchedAt, trades: current.trades };
    writeFileSync(join(stageUpstream, "UPSTREAM.json"), renderUpstreamJson(meta));
    writeFileSync(join(stageUpstream, "NOTICE.md"), renderNotice({ ...meta, vendor }));
    const newTrades = readdirSync(tree, { withFileTypes: true })
      .filter((entry) => entry.isDirectory() && existsSync(join(tree, entry.name, "claude-code", "kv")) && !current.trades.includes(entry.name))
      .map((entry) => entry.name);

    run(process.execPath, [join(stage, "scripts/generate-kitviet-pack.mjs")], stage, "generator");
    const after = await snapshotPack(stageSquads, stage);
    const additions = registryAdditions(before.previous, before, after);
    const capturedCount = Object.keys(additions.agents).length + additions.squads.length;
    if (capturedCount > 0) {
      writeFileSync(stageRegistry, appendRegistryEntries(readFileSync(stageRegistry, "utf8"), additions));
    }
    // The registry is rewritten above, so the generator output is final only after this point; check it once more.
    run(process.execPath, [join(stage, "scripts/generate-kitviet-pack.mjs"), "--check"], stage, "generator --check");
    const nodeTests = run(
      process.execPath,
      ["--test", "scripts/generate-kitviet-pack.test.mjs", "scripts/kitviet-update.test.mjs"],
      stage,
      "node tests",
    );
    const vitest = run(process.execPath, [join(repoRoot, "node_modules/vitest/vitest.mjs"), "run", "packages/core/squads", "packages/core/agents"], stage, "vitest");
    const tsc = run(join(stage, "packages/core/node_modules/.bin/tsc"), ["--noEmit"], join(stage, "packages/core"), "tsc");

    const skills = summarizeByKey(new Map(Object.entries(before.skills)), new Map(Object.entries(after.skills)));
    const agents = summarizeByKey(new Map(Object.entries(before.agents)), new Map(Object.entries(after.agents)));
    const squadTexts = summarizeByKey(new Map(Object.entries(before.squads)), new Map(Object.entries(after.squads)));
    const patches = patchesChanged(stage);
    const oldLabel = label(current.ref, current.version);
    const newLabel = label(target.sha, target.version);
    const nodeCounts = `${nodeTests.match(/ℹ pass (\d+)/)?.[1] ?? "?"} passed, ${nodeTests.match(/ℹ fail (\d+)/)?.[1] ?? "?"} failed`;
    const plainVitest = vitest.replace(/\x1b\[[0-9;]*m/g, "");
    const vitestCounts = `${plainVitest.match(/Test Files\s+(.+)/)?.[1]?.trim() ?? "?"}; ${plainVitest.match(/\n\s+Tests\s+(.+)/)?.[1]?.trim() ?? "?"}`;
    console.log(`Kit Viet upstream: ${oldLabel} -> ${newLabel}; license ${license}; ${vendor.common} common paths, ${vendor.files} vendored files.`);
    console.log(`Skills: ${skills.added.length} added (${skills.added.join(", ") || "none"}), ${skills.changed.length} changed, ${skills.removed.length} removed.`);
    console.log(`Agents: ${agents.added.length} added, ${agents.changed.length} changed, ${agents.removed.length} removed.`);
    console.log(`Squad instructions: ${squadTexts.added.length} added, ${squadTexts.changed.length} changed, ${squadTexts.removed.length} removed.`);
    console.log(`Patches changed: ${patches.length}${patches.length ? ` (${patches.join(", ")})` : ""}.`);
    console.log(`Registry: ${capturedCount} replaced texts captured.`);
    if (newTrades.length > 0) console.log(`New upstream trade folders not vendored (ignored until listed in UPSTREAM.json): ${newTrades.join(", ")}.`);
    const short = (folders) => folders.map((folder) => folder.replace(/^[^/]+\//, "")).join(", ") || "none";
    console.log(
      `Upstream skill folders: ${newFolders.length} new (${short(newFolders)}), ${goneFolders.length} removed (${short(goneFolders)}).` +
        (newFolders.length > 0 ? " A new folder is vendored, but the pack holds it only when a squad config lists it and droppedSkills does not." : ""),
    );
    console.log(`Stage checks: generator --check ok; node tests ${nodeCounts}; vitest ${vitestCounts}; tsc ${tsc.trim() === "" ? "no errors" : tsc.trim()}.`);

    ok = true;
    if (dryRun) {
      console.log("Dry run: the real files were not changed.");
      return { changed: false, dryRun: true };
    }

    const plan = [
      { from: stageUpstream, to: join(repoRoot, squadsRel, "kitviet-upstream"), kind: "dir" },
      { from: stageData, to: join(repoRoot, squadsRel, "kitviet-skills-data"), kind: "dir" },
    ];
    if (capturedCount > 0) plan.push({ from: stageRegistry, to: join(repoRoot, squadsRel, "kitviet-previous-texts.ts"), kind: "file" });
    replaceTrees(plan, join(stage, "backup"));
    console.log(`Applied ${plan.length} replacements. Nothing was committed. Review with git status and git diff --stat.`);
    return { changed: true };
  } finally {
    rmSync(tree, { recursive: true, force: true });
    if (ok) rmSync(stage, { recursive: true, force: true });
    else console.error(`Kept the stage for inspection: ${stage}`);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((err) => {
    console.error(err.message);
    process.exit(1);
  });
}
