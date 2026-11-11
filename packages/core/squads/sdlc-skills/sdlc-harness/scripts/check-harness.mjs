#!/usr/bin/env node
// Deterministic check of a repository's agent harness (AGENTS.md and what it points to).
// Plain Node, no dependencies, no LLM. Prints one JSON report on stdout.
//
//   node check-harness.mjs [--root <dir>] [--quick] [--refresh]
//
//   --root     repository to check (default: the current directory)
//   --quick    run only the blocking checks (H1 H2 H3 H4 H6 H7 H8 H10)
//   --refresh  add a `refresh` section: what changed since the provenance sha
//
// Exit code: 0 no blocking check failed, 1 a blocking check failed, 2 usage error.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const MAX_LINES = 200;
const SOFT_LINES = 150;
const MAX_BYTES = 12 * 1024;
const MUST_READ_BUDGET = 60 * 1024;
const GENERATED_DIRS = new Set([
  "node_modules", "dist", "build", "out", ".next", ".turbo", ".cache", "coverage", "target", "vendor", ".venv", "__pycache__",
]);
const MANIFEST_RE =
  /(^|\/)(package\.json|Makefile|GNUmakefile|go\.mod|\.nvmrc|\.tool-versions|\.env\.example|AGENTS\.md|docker-compose[^/]*\.ya?ml|compose[^/]*\.ya?ml)$|^\.github\/workflows\/|^supabase\/config\.toml$/;
const SECRET_PATTERNS = [
  /AKIA[0-9A-Z]{16}/,
  /-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----/,
  /\bgh[pousr]_[A-Za-z0-9]{36,}\b/,
  /\bsk-[A-Za-z0-9]{32,}\b/,
  /\bxox[abprs]-[A-Za-z0-9-]{10,}\b/,
];

// ---------------------------------------------------------------------------------------------
// Small helpers

function parseArgs(argv) {
  const args = { root: process.cwd(), quick: false, refresh: false };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--quick") args.quick = true;
    else if (a === "--refresh") args.refresh = true;
    else if (a === "--root") {
      const v = argv[++i];
      if (!v) throw new Error("--root needs a directory");
      args.root = resolve(v);
    } else throw new Error(`unknown argument: ${a}`);
  }
  return args;
}

function git(root, args) {
  try {
    return execFileSync("git", ["-C", root, ...args], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], maxBuffer: 64 * 1024 * 1024 });
  } catch {
    return null;
  }
}

function read(root, rel) {
  const p = join(root, rel);
  return existsSync(p) && statSync(p).isFile() ? readFileSync(p, "utf8") : null;
}

function readJson(root, rel) {
  const text = read(root, rel);
  if (text === null) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/** Splits markdown into fenced blocks and the prose around them. */
function splitFences(text) {
  const prose = [];
  const fences = [];
  let inFence = false;
  let marker = "";
  for (const line of text.split("\n")) {
    const m = /^\s*(`{3,}|~{3,})/.exec(line);
    if (m) {
      if (!inFence) {
        inFence = true;
        marker = m[1][0];
      } else if (m[1][0] === marker) {
        inFence = false;
      }
      prose.push("");
      continue;
    }
    if (inFence) {
      fences.push(line);
      prose.push("");
    } else prose.push(line);
  }
  return { prose: prose.join("\n"), proseLines: prose, fenceLines: fences };
}

/** Removes HTML comments from prose, keeping the line count. */
function stripComments(text) {
  return text.replace(/<!--[\s\S]*?-->/g, (m) => m.replace(/[^\n]/g, ""));
}

function codeSpans(line) {
  return [...line.matchAll(/`([^`\n]+)`/g)].map((m) => m[1]);
}

function sectionRange(lines, headingRe) {
  const ranges = [];
  for (let i = 0; i < lines.length; i++) {
    const h = /^(#{1,6})\s+(.*)$/.exec(lines[i]);
    if (!h || !headingRe.test(h[2])) continue;
    let end = lines.length;
    for (let j = i + 1; j < lines.length; j++) {
      const n = /^(#{1,6})\s/.exec(lines[j]);
      if (n && n[1].length <= h[1].length) {
        end = j;
        break;
      }
    }
    ranges.push([i, end]);
  }
  return ranges;
}

// ---------------------------------------------------------------------------------------------
// Context: everything the checks read, loaded once

function loadContext(root, config) {
  const agents = read(root, "AGENTS.md");
  const claude = read(root, "CLAUDE.md");
  const body = agents === null ? "" : stripComments(agents);
  const { prose, proseLines, fenceLines } = splitFences(body);
  const pkg = readJson(root, "package.json");
  const makeText = ["Makefile", "GNUmakefile", "makefile"].map((f) => read(root, f)).find((t) => t !== null) ?? null;
  return { root, config, agents, claude, body, prose, proseLines, fenceLines, pkg, makeText };
}

function loadConfig(root) {
  const cfg = readJson(root, "docs/sdlc/harness.json") ?? {};
  return { ignorePaths: Array.isArray(cfg.ignorePaths) ? cfg.ignorePaths.filter((p) => typeof p === "string") : [] };
}

function makeTargets(text) {
  const targets = new Set();
  if (text === null) return targets;
  for (const line of text.split("\n")) {
    if (/^\s/.test(line) || line.startsWith("#")) continue;
    const m = /^([^:=#\s][^:=#]*?)\s*:(?![=:])/.exec(line);
    if (!m) continue;
    for (const t of m[1].split(/\s+/)) if (t && !t.includes("$") && !t.includes("%")) targets.add(t);
  }
  return targets;
}

// ---------------------------------------------------------------------------------------------
// Commands in AGENTS.md (H4, H6)

const PM_BUILTINS = new Set([
  "install", "i", "add", "remove", "rm", "uninstall", "update", "up", "upgrade", "exec", "dlx", "x", "create", "init", "store", "config",
  "link", "unlink", "list", "ls", "outdated", "audit", "publish", "pack", "why", "rebuild", "prune", "dedupe", "patch", "env", "fetch",
  "import", "cache", "setup", "help", "deploy", "licenses", "doctor", "root", "bin", "recursive", "multi", "ci", "approve-builds",
  "info", "view", "login", "logout", "whoami", "version", "run", "run-script", "workspaces", "workspace", "set", "get", "explain",
]);
const NPM_SCRIPT_ALIASES = new Set(["test", "t", "tst", "start", "stop", "restart"]);

/** Every command-looking line in code spans and fenced blocks, split on shell operators. */
function commandSegments(ctx) {
  const chunks = [];
  for (const line of ctx.proseLines) for (const span of codeSpans(line)) chunks.push(span);
  for (const line of ctx.fenceLines) if (!line.trim().startsWith("#")) chunks.push(line);
  const out = [];
  for (const chunk of chunks) {
    let cwd = "";
    for (const raw of chunk.split(/&&|;|\|\||\|/)) {
      let seg = raw.trim().replace(/^\(+/, "").replace(/\)+$/, "").replace(/^\$\s+/, "").trim();
      seg = seg.replace(/^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)+/, "");
      const cd = /^cd\s+(\S+)$/.exec(seg);
      if (cd) {
        cwd = cd[1].replace(/^["']|["']$/g, "");
        continue;
      }
      if (/^(npm|pnpm|yarn|make)\b/.test(seg)) out.push({ cwd, text: seg.replace(/\s+#.*$/, "") });
    }
  }
  return out;
}

/** Resolves one command to the script or target it needs. Returns { missing } or null when it cannot be judged. */
function judgeCommand(ctx, { cwd, text }) {
  const words = text.split(/\s+/);
  const tool = words[0];
  const dir = cwd && !cwd.startsWith("/") && !cwd.includes("..") ? cwd : "";
  if (cwd && !dir) return null;
  if (tool === "make") {
    let makeDir = dir;
    const targets = [];
    for (let i = 1; i < words.length; i++) {
      const w = words[i];
      if (w === "-C" || w === "--directory") {
        makeDir = join(dir, words[++i] ?? "");
        continue;
      }
      if (w.startsWith("-") || w.includes("=") || w.startsWith("<") || w.startsWith("$") || w.startsWith(">")) continue;
      if (/^[A-Za-z0-9_.\-/]+$/.test(w)) targets.push(w);
    }
    const text2 = makeDir === "" ? ctx.makeText : ["Makefile", "GNUmakefile", "makefile"].map((f) => read(ctx.root, join(makeDir, f))).find((t) => t !== null) ?? null;
    if (targets.length === 0) return null;
    const have = makeTargets(text2);
    const missing = targets.filter((t) => !have.has(t));
    return missing.length > 0 ? { missing: missing.map((t) => `make ${t}`), reason: text2 === null ? "no Makefile" : "no such target" } : null;
  }
  // npm | pnpm | yarn
  const rest = words.slice(1);
  if (rest.some((w) => /^(--filter|-F|-C|--dir|--cwd|--prefix|--workspace|-w|-r|--recursive)(=|$)/.test(w) || w.startsWith("--filter="))) return null;
  let i = 0;
  while (i < rest.length && rest[i].startsWith("-")) i++;
  let script = rest[i];
  if (script === undefined || script.startsWith("<") || script.startsWith("$")) return null;
  let via = "";
  if (script === "run" || script === "run-script") {
    via = `${script} `;
    script = rest[i + 1];
    if (script === undefined || script.startsWith("<") || script.startsWith("$") || script.startsWith("-")) return null;
  } else if (tool === "npm") {
    if (!NPM_SCRIPT_ALIASES.has(script)) return null;
  } else if (PM_BUILTINS.has(script)) return null;
  script = script.replace(/[`'",.)]+$/, "");
  if (!/^[A-Za-z0-9:_\-.@/]+$/.test(script)) return null;
  const pkg = dir === "" ? ctx.pkg : readJson(ctx.root, join(dir, "package.json"));
  const have = pkg && typeof pkg.scripts === "object" && pkg.scripts ? Object.keys(pkg.scripts) : [];
  if (have.includes(script)) return null;
  return { missing: [`${tool} ${via}${script}`], reason: pkg ? "no such script" : "no package.json" };
}

// ---------------------------------------------------------------------------------------------
// The checks. Each returns { status: "pass" | "fail", details: string[] }.

const pass = (details = []) => ({ status: "pass", details });
const fail = (details) => ({ status: "fail", details });

function h1(ctx) {
  const details = [];
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const lines = ctx.agents.split("\n").length - (ctx.agents.endsWith("\n") ? 1 : 0);
  const bytes = Buffer.byteLength(ctx.agents);
  if (lines > MAX_LINES) details.push(`AGENTS.md has ${lines} lines (limit ${MAX_LINES})`);
  if (bytes > MAX_BYTES) details.push(`AGENTS.md is ${bytes} bytes (limit ${MAX_BYTES})`);
  if (ctx.claude === null) details.push("CLAUDE.md does not exist; it must contain @AGENTS.md");
  else {
    const rest = ctx.claude.split("\n").map((l) => l.trim()).filter((l) => l !== "");
    if (!rest.includes("@AGENTS.md")) details.push("CLAUDE.md does not import @AGENTS.md");
    else if (rest.filter((l) => l !== "@AGENTS.md").length > 1) details.push("CLAUDE.md holds more than @AGENTS.md and one sentence");
  }
  if (details.length === 0 && lines > SOFT_LINES) return pass([`AGENTS.md has ${lines} lines; aim for at most ${SOFT_LINES}`]);
  return details.length > 0 ? fail(details) : pass();
}

const PROVENANCE_RE = /Verified\s+(\d{4}-\d{2}-\d{2})\s+against\s+([0-9a-f]{7,40})\b/;

function h2(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const m = PROVENANCE_RE.exec(ctx.agents);
  if (!m) return fail(["no provenance line: add `<!-- Verified YYYY-MM-DD against <sha>. -->`"]);
  if (git(ctx.root, ["cat-file", "-e", `${m[2]}^{commit}`]) === null) return fail([`provenance sha ${m[2]} is not a commit in this repository`]);
  return pass([`verified ${m[1]} against ${m[2]}`]);
}

function pathCandidates(ctx) {
  const found = new Set();
  const add = (raw) => {
    let t = raw.trim().replace(/#.*$/, "").replace(/[),.;]+$/, "");
    if (t === "" || /\s/.test(t) || /[*<>{}$|?]/.test(t) || t.includes(":") || /^(https?|mailto)/.test(t)) return;
    if (t.startsWith("/") || t.startsWith("~") || t.startsWith("@")) return;
    const clean = t.replace(/^\.\//, "");
    const first = clean.split("/")[0];
    if (GENERATED_DIRS.has(first)) return;
    const last = clean.split("/").pop();
    const hasExt = /\.[A-Za-z0-9]{1,6}$/.test(last);
    const looksPath =
      t.startsWith("./") || t.startsWith("../") || t.endsWith("/") || (clean.includes("/") && (hasExt || existsSync(join(ctx.root, first))));
    if (looksPath) found.add(clean.replace(/\/+$/, ""));
  };
  for (const line of ctx.proseLines) {
    for (const span of codeSpans(line)) add(span);
    for (const m of line.matchAll(/\]\(([^)\s]+)\)/g)) add(m[1]);
  }
  return [...found];
}

function ignoredByGit(root, rel) {
  try {
    execFileSync("git", ["-C", root, "check-ignore", "-q", rel], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

function h3(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const missing = [];
  for (const rel of pathCandidates(ctx)) {
    if (ctx.config.ignorePaths.some((p) => rel === p || rel.startsWith(p.endsWith("/") ? p : `${p}/`))) continue;
    if (existsSync(join(ctx.root, rel))) continue;
    if (ignoredByGit(ctx.root, rel)) continue;
    missing.push(rel);
  }
  return missing.length > 0 ? fail(missing.map((p) => `path does not exist: ${p}`)) : pass();
}

function h4(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const missing = [];
  for (const seg of commandSegments(ctx)) {
    const verdict = judgeCommand(ctx, seg);
    if (verdict) for (const m of verdict.missing) missing.push(`\`${m}\`: ${verdict.reason}`);
  }
  const unique = [...new Set(missing)];
  return unique.length > 0 ? fail(unique) : pass();
}

const DONE_RE = /^\s*(?:[-*]\s+)?\**Done\**\s*[=:]\s*`([^`]+)`(.*)$/i;

function doneLines(ctx) {
  return ctx.proseLines.map((l, i) => ({ l, i })).filter(({ l }) => DONE_RE.test(l));
}

function h6(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const lines = doneLines(ctx);
  if (lines.length === 0) return fail(["no `Done = \\`<command>\\` exits 0 (evidence level L<n>)` line"]);
  if (lines.length > 1) return fail([`${lines.length} Done lines; there must be exactly one`]);
  const m = DONE_RE.exec(lines[0].l);
  const details = [];
  if (!/\bL[1-3]\b/.test(lines[0].l)) details.push("the Done line does not name the evidence level it covers (L1, L2 or L3)");
  const verdict = judgeCommand(ctx, { cwd: "", text: m[1].trim() });
  if (verdict) details.push(`the Done command does not exist: ${verdict.missing.join(", ")}`);
  return details.length > 0 ? fail(details) : pass([`Done = ${m[1]}`]);
}

const STATUS_PATTERNS = [
  /\bnot yet (?:implemented|built|done)\b/i,
  /\b(?:already |fully |partially )?implemented\b/i,
  /\bscaffold(?:ed|ing)?\b/i,
  /\bTODO\b/,
  /\bWIP\b/,
  /\bcoming soon\b/i,
  /✅|☑|\[x\]/i,
  /\|\s*(?:done|in progress|todo|planned)\s*\|/i,
  /\(\s*done\s*\)/i,
];

function h7(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const skip = new Set(doneLines(ctx).map(({ i }) => i));
  for (const [a, b] of sectionRange(ctx.proseLines, /greenfield/i)) for (let i = a; i < b; i++) skip.add(i);
  const hits = [];
  ctx.proseLines.forEach((line, i) => {
    if (skip.has(i)) return;
    if (STATUS_PATTERNS.some((re) => re.test(line))) hits.push(`line ${i + 1}: ${line.trim().slice(0, 100)}`);
  });
  return hits.length > 0 ? fail(hits.map((h) => `status wording in a rule: ${h}`)) : pass();
}

function h8(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const lines = ctx.proseLines;
  for (let i = 0; i < lines.length - 2; i++) {
    if (!/^\s*\|.*enforced\s*by.*\|/i.test(lines[i])) continue;
    if (!/^\s*\|[\s:|-]+\|\s*$/.test(lines[i + 1])) continue;
    if (/^\s*\|.*\|/.test(lines[i + 2])) return pass();
  }
  return fail(["no boundary table with an `enforced by` column and at least one row"]);
}

function h5(ctx) {
  const root = ctx.root;
  const files = [".nvmrc", ".node-version", ".tool-versions", ".python-version", "rust-toolchain", "rust-toolchain.toml", "go.mod"];
  const found = files.filter((f) => existsSync(join(root, f)));
  if (ctx.pkg && (ctx.pkg.engines || ctx.pkg.packageManager)) found.push("package.json (engines/packageManager)");
  if (found.length === 0) return fail(["no version pin: add .nvmrc, .tool-versions, or engines in package.json"]);
  const nvmrc = read(root, ".nvmrc");
  const wfDir = join(root, ".github", "workflows");
  if (nvmrc !== null && existsSync(wfDir)) {
    const major = (v) => /\d+/.exec(v)?.[0];
    const want = major(nvmrc);
    const mismatched = [];
    for (const f of readdirSync(wfDir)) {
      const text = readFileSync(join(wfDir, f), "utf8");
      for (const m of text.matchAll(/node-version:\s*['"]?([\d.x]+)['"]?/g)) if (major(m[1]) && major(m[1]) !== want) mismatched.push(`${f} uses Node ${m[1]}, .nvmrc says ${nvmrc.trim()}`);
    }
    if (mismatched.length > 0) return fail(mismatched);
  }
  return pass(found);
}

function h9(ctx) {
  const root = ctx.root;
  const files = ["CONTRIBUTING.md", ".github/PULL_REQUEST_TEMPLATE.md", ".github/pull_request_template.md", "docs/CONTRIBUTING.md", "PULL_REQUEST_TEMPLATE.md"];
  const found = files.filter((f) => existsSync(join(root, f)));
  if (found.length > 0) return pass(found);
  if (ctx.agents !== null && /commit/i.test(ctx.prose) && /(conventional|message|convention)/i.test(ctx.prose)) return pass(["commit convention in AGENTS.md"]);
  return fail(["no CONTRIBUTING, PR template, or commit convention in AGENTS.md"]);
}

function trackedFiles(root) {
  const out = git(root, ["ls-files", "-z"]);
  return out === null ? [] : out.split("\0").filter(Boolean);
}

function h10(ctx) {
  const env = read(ctx.root, "docs/sdlc/environment.md");
  if (env === null) return fail(["docs/sdlc/environment.md does not exist"]);
  const m = /^\s*(?:[-*]\s+)?\**Ensure command\**\s*[:=]\s*`([^`]+)`/im.exec(env);
  if (!m) return fail(["docs/sdlc/environment.md has no `Ensure command: \\`<command>\\`` line"]);
  const details = [];
  if (ctx.agents === null || !ctx.agents.includes("docs/sdlc/environment.md")) details.push("AGENTS.md does not point to docs/sdlc/environment.md");
  if (ctx.agents === null || !ctx.agents.includes(m[1].trim())) details.push(`AGENTS.md does not name the ensure command \`${m[1].trim()}\``);
  return details.length > 0 ? fail(details) : pass([`Ensure command: ${m[1].trim()}`]);
}

function h11(ctx) {
  const root = ctx.root;
  const details = [];
  const ignore = read(root, ".gitignore") ?? "";
  if (!/^\s*\.env(\*|\.\*|\.local)?\s*$/m.test(ignore) && !ignoredByGit(root, ".env")) details.push(".env is not ignored");
  if (!existsSync(join(root, ".env.example"))) details.push(".env.example does not exist");
  if (existsSync(join(root, ".multica")) && !ignoredByGit(root, ".multica/x")) details.push(".multica/ (squad scratch) is not ignored");
  const files = trackedFiles(root);
  if (files.some((f) => /(^|\/)\.env(\.(?!example|sample|template)[^/]+)?$/.test(f))) details.push("a real .env file is tracked");
  for (const f of files) {
    if (/\.(png|jpe?g|gif|webp|ico|zip|gz|pdf|woff2?|lock|mp4|example|sample)$/i.test(f)) continue;
    if (/(^|\/)(__tests__|fixtures?|testdata)\/|\.(test|spec)\.|_test\./.test(f)) continue;
    const p = join(root, f);
    let text;
    try {
      const st = statSync(p);
      if (!st.isFile() || st.size > 512 * 1024) continue;
      text = readFileSync(p, "utf8");
    } catch {
      continue;
    }
    if (SECRET_PATTERNS.some((re) => re.test(text))) details.push(`possible secret in tracked file: ${f}`);
  }
  return details.length > 0 ? fail(details) : pass();
}

function h12(ctx) {
  const ranges = sectionRange(ctx.proseLines, /known pitfalls?/i);
  if (ranges.length === 0) return pass(["no Known pitfalls section"]);
  const bare = [];
  for (const [a, b] of ranges) {
    for (let i = a + 1; i < b; i++) {
      const line = ctx.proseLines[i];
      if (!/^\s*[-*]\s+/.test(line)) continue;
      if (!/(\b[A-Z][A-Z0-9]+-\d+\b|\bR-\d+\b|\bRC-?\d+\b|#\d+|docs\/\S+|\bruling\b|https?:\/\/)/.test(line)) bare.push(line.trim().slice(0, 100));
    }
  }
  return bare.length > 0 ? fail(bare.map((l) => `pitfall without an evidence reference: ${l}`)) : pass();
}

function h13(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  if (/read\s+(?:when|for)\s*:/i.test(ctx.prose)) return pass(["a task index (Read when / Read for) is present"]);
  const files = new Set(["AGENTS.md"]);
  for (const line of ctx.proseLines) {
    if (!/(must[- ]read|read first|always read)/i.test(line)) continue;
    for (const span of codeSpans(line)) {
      const rel = span.replace(/^\.\//, "");
      const p = join(ctx.root, rel);
      if (!rel.includes("*") && existsSync(p) && statSync(p).isFile()) files.add(rel);
    }
  }
  let total = 0;
  for (const f of files) total += statSync(join(ctx.root, f)).size;
  return total > MUST_READ_BUDGET ? fail([`the must-read files total ${total} bytes (budget ${MUST_READ_BUDGET}); add a task index`]) : pass([`${total} bytes`]);
}

const CHECKS = [
  { id: "H1", title: "AGENTS.md exists within budget; CLAUDE.md imports it", level: "blocking", run: h1 },
  { id: "H2", title: "provenance line with a sha that exists in git", level: "blocking", run: h2 },
  { id: "H3", title: "every path AGENTS.md names exists", level: "blocking", run: h3 },
  { id: "H4", title: "every npm/pnpm/yarn/make command maps to a real script or target", level: "blocking", run: h4 },
  { id: "H5", title: "toolchain version is pinned and matches CI", level: "warning", run: h5 },
  { id: "H6", title: "exactly one Done command, with its evidence level", level: "blocking", run: h6 },
  { id: "H7", title: "no progress or status wording in the rules", level: "blocking", run: h7 },
  { id: "H8", title: "boundary table with an `enforced by` column", level: "blocking", run: h8 },
  { id: "H9", title: "commit and PR conventions exist", level: "warning", run: h9 },
  { id: "H10", title: "docs/sdlc/environment.md has an ensure command that AGENTS.md names", level: "blocking", run: h10 },
  { id: "H11", title: "scratch and secrets are ignored; no secret in tracked files", level: "warning", run: h11 },
  { id: "H12", title: "every known pitfall cites its evidence", level: "warning", run: h12 },
  { id: "H13", title: "the must-read set fits the context budget", level: "warning", run: h13 },
];

// ---------------------------------------------------------------------------------------------
// Refresh: what changed since the provenance sha

function refreshReport(ctx, report) {
  const m = ctx.agents === null ? null : PROVENANCE_RE.exec(ctx.agents);
  const head = git(ctx.root, ["rev-parse", "HEAD"])?.trim() ?? null;
  const out = { provenanceSha: m ? m[2] : null, head, manifestsChanged: [], needsRefresh: false, reasons: [] };
  if (!m) out.reasons.push("no provenance line");
  else if (head === null) out.reasons.push("not a git repository");
  else {
    const diff = git(ctx.root, ["diff", "--name-only", `${m[2]}..HEAD`]);
    if (diff === null) out.reasons.push(`cannot diff ${m[2]}..HEAD`);
    else out.manifestsChanged = diff.split("\n").filter((f) => f && MANIFEST_RE.test(f));
  }
  if (out.manifestsChanged.length > 0) out.reasons.push("manifests changed since the provenance sha");
  if (report.blocking.length > 0) out.reasons.push(`blocking checks fail: ${report.blocking.join(", ")}`);
  out.needsRefresh = out.reasons.length > 0;
  return out;
}

// ---------------------------------------------------------------------------------------------

export function checkHarness(root, { quick = false, refresh = false } = {}) {
  const ctx = loadContext(root, loadConfig(root));
  const checks = CHECKS.map((c) => {
    if (quick && c.level !== "blocking") return { id: c.id, title: c.title, level: c.level, status: "skipped", details: ["skipped by --quick"] };
    const r = c.run(ctx);
    return { id: c.id, title: c.title, level: c.level, status: r.status, details: r.details };
  });
  const blocking = checks.filter((c) => c.level === "blocking" && c.status === "fail").map((c) => c.id);
  const warnings = checks.filter((c) => c.level === "warning" && c.status === "fail").map((c) => c.id);
  const report = { root, mode: quick ? "quick" : "full", ok: blocking.length === 0, blocking, warnings, checks };
  if (refresh) report.refresh = refreshReport(ctx, report);
  return report;
}

function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
    if (!existsSync(args.root) || !statSync(args.root).isDirectory()) throw new Error(`not a directory: ${args.root}`);
  } catch (err) {
    process.stderr.write(`check-harness: ${err.message}\nusage: check-harness.mjs [--root <dir>] [--quick] [--refresh]\n`);
    process.exit(2);
  }
  const report = checkHarness(args.root, args);
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  process.exit(report.ok ? 0 : 1);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
