#!/usr/bin/env node
// Deterministic check of a repository's agent harness (AGENTS.md and what it points to).
// Plain Node, no dependencies, no LLM. Prints one JSON report on stdout.
//
//   node check-harness.mjs [--root <dir>] [--quick] [--cert] [--run [--timeout <s>]] [--refresh]
//
//   --root     repository to check (default: the current directory)
//   --quick    run only the gate checks (H1 H3 H4): the harness is missing or points at things that do not exist
//   --cert     a cert-level failure also blocks: the conditions for certifying the Harness stage
//   --run      execute the Ensure command (E1) and the Done command (E2); implies --cert. Architect only, on the squad's own repo
//   --timeout  seconds each executed command may take (default 900, or `timeoutSeconds` in docs/sdlc/harness.json)
//   --write-cert        with --run: when certified, write docs/sdlc/harness-cert.json (sha, date, executed commands, footprint)
//   --accept <ID>=<A-n> record that cert-level check <ID> is lowered on purpose, by assumption <A-n>; repeatable
//   --refresh  add a `refresh` section: what changed since the certificate
//
// Levels: gate (stops the Developer), cert (needed to certify the stage), warning (reported only).
// Exit code: 0 nothing blocks in this mode, 1 something blocks, 2 usage error.

import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// Provisional size limits. Hard: past this the stage is not certified (the Codex default cap for the whole
// instruction chain is 32 KiB). Soft: past this a warning. The target is 80 lines.
const MAX_LINES = 200;
const SOFT_LINES = 120;
const MAX_BYTES = 24 * 1024;
const SOFT_BYTES = 8 * 1024;
const MUST_READ_BUDGET = 32 * 1024;
const GENERATED_DIRS = new Set([
  "node_modules", "dist", "build", "out", ".next", ".turbo", ".cache", "coverage", "target", "vendor", ".venv", "__pycache__",
]);
const MANIFEST_RE =
  /^docs\/sdlc\/environment\.md$|(^|\/)(package\.json|Makefile|GNUmakefile|go\.mod|\.nvmrc|\.tool-versions|\.env\.example|AGENTS\.md|docker-compose[^/]*\.ya?ml|compose[^/]*\.ya?ml)$|^\.github\/workflows\/|^supabase\/config\.toml$/;
const SECRET_PATTERNS = [
  /AKIA[0-9A-Z]{16}/,
  /-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----/,
  /\bgh[pousr]_[A-Za-z0-9]{36,}\b/,
  /\bsk-[A-Za-z0-9]{32,}\b/,
  /\bxox[abprs]-[A-Za-z0-9-]{10,}\b/,
];
// Words that only appear in a grader's or a QA's output. If they reach the files an agent reads, the agent can copy the answer (H17).
const LEAK_MARKERS = ["QA GO", "Security Reviewer verdict", "n_passed", "pass_rate", "test_pass_rate", "grade-task.mjs"];
const LEAK_NOTE = "This is a leak, not a false alarm. Find the read path that brought it into the instructions.";
// Budget for the rules and pitfalls sections together. Past 60% of it, H14 warns and suggests moving a rule into a check.
const FOOTPRINT_BUDGET_BYTES = 8 * 1024;
const PITFALL_FIELDS_REQUIRED = ["cue", "lesson", "evidence", "confidence", "scope"];
const PITFALL_CONFIDENCE = ["high", "medium", "low"];
const PITFALL_SCOPE = ["repo", "stack", "workspace-fact"];
const EVIDENCE_RE = /(\b[A-Z][A-Z0-9]+-\d+\b|\bR-\d+\b|\bRC-?\d+\b|#\d+|docs\/\S+|\bruling\b|https?:\/\/)/;

// ---------------------------------------------------------------------------------------------
// Small helpers

function parseArgs(argv) {
  const args = { root: process.cwd(), quick: false, cert: false, run: false, timeout: null, writeCert: false, accept: [], refresh: false };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--quick") args.quick = true;
    else if (a === "--cert") args.cert = true;
    else if (a === "--run") args.run = true;
    else if (a === "--write-cert") args.writeCert = true;
    else if (a === "--accept") {
      const m = /^(H\d+|E\d+)=(\S+)$/.exec(argv[++i] ?? "");
      if (!m) throw new Error("--accept needs <check id>=<assumption id>, for example H6=A-3");
      args.accept.push({ check: m[1], assumption: m[2] });
    }
    else if (a === "--timeout") {
      const n = Number(argv[++i]);
      if (!Number.isFinite(n) || n <= 0) throw new Error("--timeout needs a number of seconds");
      args.timeout = n;
    }
    else if (a === "--refresh") args.refresh = true;
    else if (a === "--root") {
      const v = argv[++i];
      if (!v) throw new Error("--root needs a directory");
      args.root = resolve(v);
    } else throw new Error(`unknown argument: ${a}`);
  }
  if (args.run && args.quick) throw new Error("--run cannot be combined with --quick");
  if (args.writeCert && !args.run) throw new Error("--write-cert needs --run");
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
  return {
    ignorePaths: Array.isArray(cfg.ignorePaths) ? cfg.ignorePaths.filter((p) => typeof p === "string") : [],
    timeoutSeconds: typeof cfg.timeoutSeconds === "number" && cfg.timeoutSeconds > 0 ? cfg.timeoutSeconds : 900,
    markers: Array.isArray(cfg.markers) ? cfg.markers.filter((m) => typeof m === "string" && m.trim() !== "") : [],
    // An exception needs a reason and an approver; without both it is ignored and the marker still counts.
    markerExceptions: Array.isArray(cfg.markerExceptions)
      ? cfg.markerExceptions.filter(
          (e) => e && typeof e.path === "string" && typeof e.marker === "string" && typeof e.reason === "string" && e.reason.trim() !== "" && typeof e.approver === "string" && e.approver.trim() !== "",
        )
      : [],
  };
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
  // Only these four tools are resolved; any other command (go test, pytest, sh scripts/x.sh) is left to the executed checks.
  if (!["make", "npm", "pnpm", "yarn"].includes(tool)) return null;
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
  if (ctx.claude === null) details.push("CLAUDE.md does not exist; it must contain @AGENTS.md");
  else {
    const rest = ctx.claude.split("\n").map((l) => l.trim()).filter((l) => l !== "");
    if (!rest.includes("@AGENTS.md")) details.push("CLAUDE.md does not import @AGENTS.md");
    else if (rest.filter((l) => l !== "@AGENTS.md").length > 1) details.push("CLAUDE.md holds more than @AGENTS.md and one sentence");
  }
  return details.length > 0 ? fail(details) : pass();
}

function lineCount(text) {
  return text.split("\n").length - (text.endsWith("\n") ? 1 : 0);
}

/** Size of AGENTS.md. Past the hard limit it blocks certification; past the soft one it only warns. */
function h14(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const lines = lineCount(ctx.agents);
  const bytes = Buffer.byteLength(ctx.agents);
  const over = [];
  if (lines > MAX_LINES) over.push(`AGENTS.md has ${lines} lines (hard limit ${MAX_LINES})`);
  if (bytes > MAX_BYTES) over.push(`AGENTS.md is ${bytes} bytes (hard limit ${MAX_BYTES})`);
  if (over.length > 0) return { status: "fail", level: "cert", details: over };
  if (lines > SOFT_LINES) over.push(`AGENTS.md has ${lines} lines; aim for at most 80, warning above ${SOFT_LINES}`);
  if (bytes > SOFT_BYTES) over.push(`AGENTS.md is ${bytes} bytes; warning above ${SOFT_BYTES}`);
  const rulesBytes = sectionSizes(ctx).filter((s) => isRulesOrPitfalls(s.name)).reduce((n, s) => n + s.bytes, 0);
  if (rulesBytes > 0.6 * FOOTPRINT_BUDGET_BYTES) {
    over.push(
      `Rules and Known pitfalls take ${rulesBytes} bytes, past 60% of the ${FOOTPRINT_BUDGET_BYTES}-byte budget: move a rule into a check (lint, test, CI) or into a skill with a trigger`,
    );
  }
  return over.length > 0 ? { status: "fail", level: "warning", details: over } : pass([`${lines} lines, ${bytes} bytes`]);
}

const PROVENANCE_RE = /Verified\s+(\d{4}-\d{2}-\d{2})\s+against\s+([0-9a-f]{7,40})\b/;
const CERT_PATH = "docs/sdlc/harness-cert.json";

/** The certificate, or the v1 provenance line in AGENTS.md as a fallback. */
function loadCert(ctx) {
  const raw = read(ctx.root, CERT_PATH);
  if (raw !== null) {
    let cert = null;
    try {
      cert = JSON.parse(raw);
    } catch {
      return { error: `${CERT_PATH} is not valid JSON` };
    }
    if (!cert || cert.schema !== 1 || typeof cert.sha !== "string" || typeof cert.date !== "string") {
      return { error: `${CERT_PATH} does not match schema 1 (needs schema, sha, date)` };
    }
    return { sha: cert.sha, date: cert.date, source: CERT_PATH, cert };
  }
  const m = ctx.agents === null ? null : PROVENANCE_RE.exec(ctx.agents);
  return m ? { sha: m[2], date: m[1], source: "AGENTS.md provenance line" } : null;
}

function h2(ctx) {
  const c = loadCert(ctx);
  if (c === null) return fail([`no certificate: run check-harness --cert --run --write-cert (writes ${CERT_PATH})`]);
  if (c.error) return fail([c.error]);
  if (git(ctx.root, ["cat-file", "-e", `${c.sha}^{commit}`]) === null) return fail([`certificate sha ${c.sha} is not a commit in this repository`]);
  return pass([`certified ${c.date} against ${c.sha} (${c.source})`]);
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
  /\b(?:is|are|was|were|has been|have been|already|fully|partially)\s+implemented\b/i,
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

const LIST_ITEM_RE = /^\s*(?:[-*]|\d+\.)\s+/;
const RULES_HEADING_RE = /^(rules?|policy|policies)\b/i;

/** The list items of every section whose heading matches, as { line, text }. */
function sectionItems(ctx, headingRe) {
  const items = [];
  for (const [a, b] of sectionRange(ctx.proseLines, headingRe)) {
    for (let i = a + 1; i < b; i++) if (LIST_ITEM_RE.test(ctx.proseLines[i])) items.push({ line: i + 1, text: ctx.proseLines[i].trim() });
  }
  return items;
}

const ENFORCED_RE = /enforced\s+by\s*:\s*(.+?)\s*$/i;

function h8(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const details = [];
  for (const { line, text } of sectionItems(ctx, RULES_HEADING_RE)) {
    const m = ENFORCED_RE.exec(text);
    if (!m) {
      details.push(`line ${line}: rule without "Enforced by: <target>" (write "prose" if nothing enforces it)`);
      continue;
    }
    if (/^`?prose`?\.?$/i.test(m[1])) continue;
    for (const span of codeSpans(m[1])) {
      const cands = pathCandidates({ ...ctx, proseLines: [`\`${span}\``] });
      const missingPath = cands.find((c) => !existsSync(join(ctx.root, c)) && !ignoredByGit(ctx.root, c));
      if (missingPath) details.push(`line ${line}: "Enforced by" names a path that does not exist: ${missingPath}`);
      else if (cands.length === 0) {
        const verdict = /^(npm|pnpm|yarn|make)\b/.test(span) ? judgeCommand(ctx, { cwd: "", text: span }) : null;
        if (verdict) details.push(`line ${line}: "Enforced by" names a command that does not exist: ${verdict.missing.join(", ")}`);
      }
    }
  }
  return details.length > 0 ? fail(details) : pass();
}

/** A pointer to a document must say when to read it. A bare path or link is not enough. */
function h15(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const verify = new Set();
  for (const [a, b] of sectionRange(ctx.proseLines, /verify|running and verifying/i)) for (let i = a; i < b; i++) verify.add(i);
  const bare = /^\s*(?:[-*]|\d+\.)\s+(?:read\s+)?(?:`[^`]+`|\[[^\]]+\]\([^)]+\))[.\s]*$/i;
  const details = [];
  ctx.proseLines.forEach((line, i) => {
    if (!verify.has(i) && bare.test(line)) details.push(`line ${i + 1}: pointer without a condition: ${line.trim().slice(0, 100)}`);
  });
  return details.length > 0 ? fail(details) : pass();
}

const OVERVIEW_HEADING_RE = /^#{1,6}\s*(overview|architecture|project structure|tech(?:nology)? stack|directory (?:layout|structure)|about)\b/i;

/** Signs of a repository overview, which the agent can get from the code and the README. */
function h16(ctx) {
  if (ctx.agents === null) return fail(["AGENTS.md does not exist"]);
  const skip = new Set();
  for (const [a, b] of sectionRange(ctx.proseLines, /greenfield/i)) for (let i = a; i < b; i++) skip.add(i);
  const details = [];
  const raw = stripComments(ctx.agents).split("\n");
  raw.forEach((line, i) => {
    if (!skip.has(i) && /[├└]──/.test(line)) details.push(`line ${i + 1}: directory tree`);
  });
  let run = 0;
  ctx.proseLines.forEach((line, i) => {
    if (!skip.has(i) && OVERVIEW_HEADING_RE.test(line)) details.push(`line ${i + 1}: overview-style heading: ${line.trim()}`);
    const narrative = line.trim() !== "" && !skip.has(i) && !/^\s*(?:[-*>|#]|\d+\.)/.test(line);
    run = narrative ? run + 1 : 0;
    if (run === 3) details.push(`line ${i - 1}: a paragraph of three or more lines; AGENTS.md holds rules and pointers, not narrative`);
  });
  return details.length > 0 ? fail(details) : pass();
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
  const publicOnly = (f) => {
    const text = read(root, f);
    const keys = (text ?? "").split("\n").map((l) => l.trim()).filter((l) => l !== "" && !l.startsWith("#")).map((l) => l.split("=")[0].replace(/^export\s+/, ""));
    return keys.length > 0 && keys.every((k) => /^(EXPO_PUBLIC_|NEXT_PUBLIC_|VITE_|PUBLIC_)/.test(k));
  };
  if (files.some((f) => /(^|\/)\.env(\.(?!example|sample|template)[^/]+)?$/.test(f) && !publicOnly(f))) details.push("a real .env file is tracked");
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
  const ranges = sectionRange(ctx.proseLines, /known pitfalls?|gotchas?/i);
  if (ranges.length === 0) return pass(["no Known pitfalls section"]);
  const details = [];
  const today = new Date().toISOString().slice(0, 10);
  for (const [a, b] of ranges) {
    // A pitfall is a top-level bullet. Its fields are the indented `- **field**: value` bullets under it.
    const pitfalls = [];
    for (let i = a + 1; i < b; i++) {
      const line = ctx.proseLines[i];
      if (/^[-*]\s+/.test(line)) {
        pitfalls.push({ text: line.trim(), fields: new Map() });
        continue;
      }
      const field = /^\s+[-*]\s+\*\*([a-z_]+)\*\*\s*:\s*(.*)$/.exec(line);
      if (field && pitfalls.length > 0) pitfalls[pitfalls.length - 1].fields.set(field[1], field[2].trim());
    }
    for (const p of pitfalls) {
      const evidenceField = p.fields.get("evidence") ?? "";
      if (!EVIDENCE_RE.test(p.text) && !EVIDENCE_RE.test(evidenceField)) details.push(`pitfall without an evidence reference: ${p.text.slice(0, 100)}`);
      const missing = PITFALL_FIELDS_REQUIRED.filter((f) => !p.fields.get(f));
      if (missing.length > 0) details.push(`pitfall missing field(s) ${missing.join(", ")}: ${p.text.slice(0, 100)}`);
      const confidence = p.fields.get("confidence");
      if (confidence && !PITFALL_CONFIDENCE.includes(confidence)) details.push(`pitfall confidence must be high, medium or low, not "${confidence}": ${p.text.slice(0, 100)}`);
      const scope = p.fields.get("scope");
      if (scope && !PITFALL_SCOPE.includes(scope)) details.push(`pitfall scope must be repo, stack or workspace-fact, not "${scope}": ${p.text.slice(0, 100)}`);
      const until = p.fields.get("valid_until");
      if (until && /^\d{4}-\d{2}-\d{2}$/.test(until) && until < today) details.push(`pitfall valid_until ${until} has passed (today ${today}): re-verify it or delete it: ${p.text.slice(0, 100)}`);
    }
  }
  return details.length > 0 ? fail(details) : pass();
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

/** Every file under a directory, relative to the root. Generated and vendored folders are skipped. */
function walkFiles(root, rel, out = []) {
  const abs = join(root, rel);
  if (!existsSync(abs) || !statSync(abs).isDirectory()) return out;
  for (const entry of readdirSync(abs, { withFileTypes: true })) {
    const child = `${rel}/${entry.name}`;
    if (entry.isDirectory()) {
      if (!GENERATED_DIRS.has(entry.name)) walkFiles(root, child, out);
    } else if (entry.isFile()) out.push(child);
  }
  return out;
}

/** Sizes of the `## ` sections of AGENTS.md, in order. */
function sectionSizes(ctx) {
  const out = [];
  let current = null;
  for (const line of ctx.body.split("\n")) {
    const h = /^##\s+(.*)$/.exec(line);
    if (h) {
      current = { name: h[1].trim(), lines: 0, bytes: 0 };
      out.push(current);
    }
    if (current) {
      current.lines += 1;
      current.bytes += Buffer.byteLength(`${line}\n`);
    }
  }
  return out;
}

const isRulesOrPitfalls = (name) => RULES_HEADING_RE.test(name) || /known pitfalls?|gotchas?/i.test(name);

/** Markdown files an agent reads as instructions. Code is not scanned, because the marker list itself lives in code. */
function instructionFiles(root) {
  const files = walkFiles(root, "docs/sdlc").filter((f) => f.endsWith(".md"));
  for (const dir of SKILL_DIRS) files.push(...walkFiles(root, dir).filter((f) => f.endsWith(".md")));
  const builtin = join(root, "catalog/skills/builtin");
  if (existsSync(builtin)) {
    for (const name of readdirSync(builtin).filter((n) => n.startsWith("sdlc-"))) {
      files.push(...walkFiles(root, `catalog/skills/builtin/${name}`).filter((f) => f.endsWith(".md")));
    }
  }
  return files;
}

/** H17: a grader's or QA's verdict in a file an agent reads is a leak: the agent can copy the answer. */
function h17(ctx) {
  const files = ["AGENTS.md", "CLAUDE.md"].filter((f) => existsSync(join(ctx.root, f)));
  files.push(...instructionFiles(ctx.root));
  const markers = [...LEAK_MARKERS, ...ctx.config.markers];
  const details = [];
  for (const rel of files) {
    const text = read(ctx.root, rel);
    if (text === null) continue;
    text.split("\n").forEach((line, i) => {
      for (const marker of markers) {
        const at = line.indexOf(marker);
        if (at < 0) continue;
        if (ctx.config.markerExceptions.some((e) => e.path === rel && e.marker === marker)) continue;
        const snippet = line.slice(Math.max(0, at - 60), at + marker.length + 60).trim();
        details.push(`${rel}:${i + 1}: "${snippet}" contains the marker "${marker}". ${LEAK_NOTE}`);
      }
    });
  }
  return details.length > 0 ? fail(details) : pass([`${files.length} instruction files scanned`]);
}

const SKILL_DIRS = [".claude/skills", ".kiro/skills", ".agents/skills", "skills"];

/** The lines of AGENTS.md that sit under Read when (or Read for): a heading, or a `Read when:` line and the list under it. */
function readWhenLines(ctx) {
  const set = new Set();
  for (const [a, b] of sectionRange(ctx.proseLines, /read when|read for/i)) for (let i = a; i < b; i++) set.add(i);
  ctx.proseLines.forEach((line, i) => {
    if (!/read\s+(?:when|for)\s*:/i.test(line)) return;
    for (let j = i; j < ctx.proseLines.length && ctx.proseLines[j].trim() !== ""; j++) set.add(j);
  });
  return set;
}

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** H18: a skill or runbook that nothing names is never called. A pointer in AGENTS.md outside Read when has no condition. */
function h18(ctx) {
  const targets = [];
  for (const dir of SKILL_DIRS) {
    const abs = join(ctx.root, dir);
    if (!existsSync(abs) || !statSync(abs).isDirectory()) continue;
    for (const name of readdirSync(abs).sort()) {
      const rel = `${dir}/${name}/SKILL.md`;
      if (read(ctx.root, rel) !== null) targets.push({ rel, name: new RegExp(`(?<![\\w-])${escapeRe(name)}(?![\\w-])`) });
    }
  }
  for (const rel of walkFiles(ctx.root, "docs").sort()) {
    if (!/\.md$/.test(rel) || !/runbook/i.test(rel)) continue;
    const base = rel.split("/").pop();
    targets.push({ rel, name: new RegExp(`(?<![\\w-])${escapeRe(base)}(?![\\w-])`) });
  }
  if (targets.length === 0) return pass(["no skills or runbooks in the repository"]);
  if (ctx.agents === null) return pass();
  const sources = instructionFiles(ctx.root).map((rel) => ({ rel, text: read(ctx.root, rel) ?? "" }));
  const readWhen = readWhenLines(ctx);
  const details = [];
  for (const t of targets) {
    const hits = ctx.proseLines.map((line, i) => ({ line, i })).filter(({ line }) => t.name.test(line));
    const others = sources.filter((s) => s.rel !== t.rel && t.name.test(s.text));
    const outside = hits.filter(({ line, i }) => !readWhen.has(i) && !/\b(when|if|before|after|unless)\b/i.test(line));
    if (hits.length === 0 && others.length === 0) {
      details.push(`${t.rel} is named nowhere in AGENTS.md, CLAUDE.md, docs/sdlc or another skill, so it will never be called. Name it under Read when with its condition, or delete it.`);
    } else if (hits.length > 0 && outside.length === hits.length) {
      details.push(`${t.rel} is named at AGENTS.md line ${outside[0].i + 1} outside Read when, with no condition. Move the pointer under Read when and state when to read it.`);
    }
  }
  return details.length > 0 ? fail(details) : pass([`${targets.length} skills or runbooks checked`]);
}

const CHECKS = [
  { id: "H1", title: "AGENTS.md exists; CLAUDE.md imports it", level: "gate", run: h1 },
  { id: "H2", title: "provenance with a sha that exists in git", level: "warning", run: h2 },
  { id: "H3", title: "every path AGENTS.md names exists", level: "gate", run: h3 },
  { id: "H4", title: "every npm/pnpm/yarn/make command maps to a real script or target", level: "gate", run: h4 },
  { id: "H5", title: "toolchain version is pinned and matches CI", level: "warning", run: h5 },
  { id: "H6", title: "exactly one Done command, with its evidence level", level: "cert", run: h6 },
  { id: "H7", title: "no progress or status wording in the rules", level: "cert", run: h7 },
  { id: "H8", title: "every rule names what enforces it (or says prose)", level: "warning", run: h8 },
  { id: "H9", title: "commit and PR conventions exist", level: "warning", run: h9 },
  { id: "H10", title: "docs/sdlc/environment.md has an ensure command that AGENTS.md names", level: "cert", run: h10 },
  { id: "H11", title: "scratch and secrets are ignored; no secret in tracked files", level: "warning", run: h11 },
  { id: "H12", title: "every known pitfall cites its evidence and carries its fields", level: "warning", run: h12 },
  { id: "H13", title: "the must-read set fits the context budget", level: "warning", run: h13 },
  { id: "H14", title: "AGENTS.md size and section budget", level: "cert", run: h14 },
  { id: "H15", title: "every pointer to a document says when to read it", level: "warning", run: h15 },
  { id: "H16", title: "no repository overview in AGENTS.md", level: "warning", run: h16 },
  { id: "H17", title: "instruction files hold no grader or QA output (leak tripwire)", level: "warning", run: h17 },
  { id: "H18", title: "every skill or runbook is named under Read when, with its condition", level: "warning", run: h18 },
];

// ---------------------------------------------------------------------------------------------
// Refresh: what changed since the provenance sha

function refreshReport(ctx, report) {
  const c = loadCert(ctx);
  const head = git(ctx.root, ["rev-parse", "HEAD"])?.trim() ?? null;
  const out = { certSha: c && !c.error ? c.sha : null, certDate: c && !c.error ? c.date : null, head, staleDays: null, manifestsChanged: [], needsRefresh: false, reasons: [] };
  if (c === null) out.reasons.push("no certificate");
  else if (c.error) out.reasons.push(c.error);
  else if (head === null) out.reasons.push("not a git repository");
  else {
    const days = Math.floor((Date.now() - Date.parse(c.date)) / 86400000);
    if (Number.isFinite(days)) out.staleDays = Math.max(0, days);
    let ancestor = true;
    try {
      execFileSync("git", ["-C", ctx.root, "merge-base", "--is-ancestor", c.sha, "HEAD"], { stdio: "ignore" });
    } catch {
      ancestor = false;
    }
    if (!ancestor) out.reasons.push(`certificate sha ${c.sha} is not an ancestor of HEAD`);
    else {
      const diff = git(ctx.root, ["diff", "--name-only", `${c.sha}..HEAD`]);
      if (diff === null) out.reasons.push(`cannot diff ${c.sha}..HEAD`);
      else out.manifestsChanged = diff.split("\n").filter((f) => f && MANIFEST_RE.test(f));
    }
  }
  if (out.manifestsChanged.length > 0) out.reasons.push("manifests changed since the certificate");
  if (report.blocking.length > 0) out.reasons.push(`blocking checks fail: ${report.blocking.join(", ")}`);
  out.needsRefresh = out.reasons.length > 0;
  return out;
}

function footprint(ctx) {
  if (ctx.agents === null) return { lines: 0, bytes: 0, instructions: 0, proseRules: 0, sections: [] };
  const instructions = ctx.proseLines.filter((l) => LIST_ITEM_RE.test(l)).length;
  const proseRules = sectionItems(ctx, RULES_HEADING_RE).filter(({ text }) => {
    const m = ENFORCED_RE.exec(text);
    return !m || /^`?prose`?\.?$/i.test(m[1]);
  }).length;
  return { lines: lineCount(ctx.agents), bytes: Buffer.byteLength(ctx.agents), instructions, proseRules, sections: sectionSizes(ctx) };
}

/** Why each check exists, in one line. A failing check shows it next to what is wrong (W8). */
const WHY = {
  H1: "Every tool must read the same rules, so CLAUDE.md imports AGENTS.md and holds nothing else.",
  H2: "A certificate shows which commit was checked. Without a valid one, nobody knows which harness ran.",
  H3: "An agent that follows a path that does not exist loses its first step.",
  H4: "An agent that runs a command that does not exist fails at once and cannot tell why.",
  H5: "An unpinned toolchain makes the same command pass on one machine and fail on another.",
  H6: "The Done command is the only proof that the work is finished. Exactly one line makes it unambiguous.",
  H7: "Progress wording goes stale and states a project status that the agent cannot check.",
  H8: "A rule that nothing enforces is only a wish. Naming the enforcer shows when it should become code.",
  H9: "Without a commit or PR convention, the history cannot be read and reviews have no shape.",
  H10: "The Ensure command brings the environment up. Without it the agent starts from a broken machine.",
  H11: "A secret or local scratch in git leaks values and clutters every commit.",
  H12: "A pitfall without evidence cannot be checked, and one without fields cannot be pruned when its cause is gone.",
  H13: "Every must-read file is read on every run. A large set spends the context before the work starts.",
  H14: "Long instruction files are followed less reliably and cost tokens on every run.",
  H15: "A pointer without a condition is read on every run or never. The condition makes it load when it matters.",
  H16: "An overview repeats what the code and the README already say, and goes stale.",
  H17: "A grader's or QA's verdict in an instruction file lets the agent copy the answer instead of solving the task.",
  H18: "A skill that nothing names is never called. Its work is lost and its files only add weight.",
  E1: "The Ensure command is the first thing an agent runs. If it fails, nothing after it can be trusted.",
  E2: "The Done command is the proof. An exit code other than 0 means the work is not verified.",
};

/** What to change for a failing check, written for the agent that has to change it. */
const FIX = {
  H1: "Make CLAUDE.md hold the line @AGENTS.md, and at most one sentence after it.",
  H2: "Run `check-harness --cert --run --write-cert` to write a certificate for a commit that exists.",
  H3: "Create the named file, or delete the line in AGENTS.md that names it.",
  H4: "Add the script or make target to package.json or the Makefile, or delete the command from AGENTS.md.",
  H5: "Pin the toolchain (.nvmrc, .tool-versions or engines) and make CI use the same major version.",
  H6: "Keep exactly one line: Done = <command> exits 0 (evidence level L1, L2 or L3). Make sure the command exists.",
  H7: "Rewrite the line as an instruction, and delete the progress wording.",
  H8: "Append `Enforced by: <lint rule | test | CI job | hook | prose>` to the rule, with a path or command that exists.",
  H9: "Add CONTRIBUTING.md, a PR template, or a commit convention to AGENTS.md.",
  H10: "Add a line 'Ensure command: <command>' to docs/sdlc/environment.md, and name that file and command in AGENTS.md.",
  H11: "Add `.env` and `.multica/` to .gitignore, add .env.example with names only, and remove the secret from the tracked file.",
  H12: "Under the pitfall, add the missing fields: cue, lesson, evidence, confidence (high, medium or low), scope (repo, stack or workspace-fact). Re-verify or delete an expired valid_until.",
  H13: "Add a `Read when:` index, or make the must-read files smaller.",
  H14: "Shorten AGENTS.md: move status and narrative to docs/, and keep Verify, Rules, Known pitfalls and Read when.",
  H15: "Say when to read the document, for example: Changing the API client: read src/lib/api.ts.",
  H16: "Delete the tree, the overview heading or the narrative paragraph. Keep rules and pointers.",
  H17: "Remove the verdict or grade text from the file, or state the rule it implies. Do not paste results into instructions.",
  H18: "Name the skill or runbook under Read when with its condition, or delete it.",
  E1: "Fix the Ensure command so it exits 0 on a clean machine, then run `--run` again.",
  E2: "Fix the Done command, or commit the open paths, then run `--run` again.",
};

/** W8: each failing check lists its findings as { what, why, fix }. `details` stays for readers of the older format. */
function withFindings(check) {
  if (check.status !== "fail") return check;
  const findings = check.details.map((what) => ({ what, why: WHY[check.id], fix: FIX[check.id] }));
  return { ...check, findings };
}

/** The git blob id of a text file, as `git hash-object` computes it. The Lead's Harness fingerprint uses it for AGENTS.md. */
const blobId = (text) => createHash("sha1").update(`blob ${Buffer.byteLength(text)}\0`).update(text).digest("hex");

function summarize(ctx, checks, { quick, cert, run, accept = [] }) {
  const failed = (level) => checks.filter((c) => c.level === level && c.status === "fail").map((c) => c.id);
  const accepted = new Set(accept.map((a) => a.check));
  const gate = failed("gate");
  const certAll = failed("cert");
  const certFailures = certAll.filter((id) => !accepted.has(id));
  const blocking = quick || !(cert || run) ? gate : [...gate, ...certFailures];
  const executedOk = !run || ["E1", "E2"].every((id) => checks.find((c) => c.id === id)?.status === "pass" || accepted.has(id));
  return {
    root: ctx.root,
    agentsBlob: ctx.agents === null ? null : blobId(ctx.agents).slice(0, 12),
    mode: quick ? "quick" : run ? "run" : cert ? "cert" : "full",
    ok: gate.length === 0,
    certified: !quick && gate.length === 0 && certFailures.length === 0 && executedOk,
    blocking,
    cert: certFailures,
    accepted: certAll.filter((id) => accepted.has(id)),
    warnings: failed("warning"),
    footprint: footprint(ctx),
    checks,
  };
}

export function checkHarness(root, { quick = false, cert = false, run = false, refresh = false, accept = [] } = {}) {
  const ctx = loadContext(root, loadConfig(root));
  const checks = CHECKS.map((c) => {
    if (quick && c.level !== "gate") return { id: c.id, title: c.title, level: c.level, status: "skipped", details: ["skipped by --quick"] };
    const r = c.run(ctx);
    return withFindings({ id: c.id, title: c.title, level: r.level ?? c.level, status: r.status, details: r.details });
  });
  const report = summarize(ctx, checks, { quick, cert, run, accept });
  if (refresh) report.refresh = refreshReport(ctx, report);
  return report;
}

// ---------------------------------------------------------------------------------------------
// Executed checks (E1, E2): prove that the Ensure and Done commands run

const TAIL_LINES = 20;

function tailOf(output) {
  return output
    .split("\n")
    .filter((l) => l.trim() !== "")
    .slice(-TAIL_LINES)
    .map((l) => (SECRET_PATTERNS.some((re) => re.test(l)) ? "[redacted: matches a secret pattern]" : l.slice(0, 300)));
}

/** Runs one shell command at the repo root with a time limit. A timeout kills the whole process group. */
function runCommand(root, command, seconds) {
  return new Promise((resolveRun) => {
    process.stderr.write(`check-harness: running \`${command}\` (limit ${seconds}s)\n`);
    const started = Date.now();
    let out = "";
    let timedOut = false;
    const child = spawn("sh", ["-c", command], { cwd: root, env: { ...process.env, CI: "1" }, detached: true, stdio: ["ignore", "pipe", "pipe"] });
    const collect = (d) => {
      out += d.toString();
      if (out.length > 200000) out = out.slice(-100000);
    };
    child.stdout.on("data", collect);
    child.stderr.on("data", collect);
    const timer = setTimeout(() => {
      timedOut = true;
      try {
        process.kill(-child.pid, "SIGKILL");
      } catch {
        child.kill("SIGKILL");
      }
    }, seconds * 1000);
    const finish = (exit) => {
      clearTimeout(timer);
      resolveRun({ command, exit: timedOut ? null : exit, timedOut, seconds: Math.round((Date.now() - started) / 100) / 10, tail: tailOf(out) });
    };
    child.on("error", (err) => {
      out += String(err.message);
      finish(127);
    });
    child.on("close", (code) => finish(code ?? 1));
  });
}

function ensureCommand(root) {
  const env = read(root, "docs/sdlc/environment.md");
  const m = env === null ? null : /^\s*(?:[-*]\s+)?\**Ensure command\**\s*[:=]\s*`([^`]+)`/im.exec(env);
  return m ? m[1].trim() : null;
}

function describeRun(label, r, extra = []) {
  if (r.timedOut) return [`${label} did not finish within ${r.seconds}s and was killed`, ...extra];
  return r.exit === 0 ? [`exit 0 in ${r.seconds}s`, ...extra] : [`${label} exited ${r.exit}`, ...r.tail.slice(-5), ...extra];
}

export async function runExecutedChecks(root, report, { timeout, accept = [] } = {}) {
  const ctx = loadContext(root, loadConfig(root));
  const seconds = timeout ?? ctx.config.timeoutSeconds;
  const status = (id) => report.checks.find((c) => c.id === id)?.status;
  const executed = {};
  const add = (id, title, ok, details) => report.checks.push(withFindings({ id, title, level: "cert", status: ok ? "pass" : "fail", details }));

  const ensure = status("H10") === "pass" ? ensureCommand(root) : null;
  if (ensure === null) add("E1", "the Ensure command exits 0", false, ["not run: H10 fails, so there is no Ensure command to run"]);
  else {
    const r = await runCommand(root, ensure, seconds);
    executed.ensure = { command: r.command, exit: r.exit, seconds: r.seconds, tail: r.tail };
    add("E1", "the Ensure command exits 0", r.exit === 0, describeRun("Ensure", r));
  }

  const doneLine = status("H6") === "pass" ? doneLines(ctx)[0] : undefined;
  if (!doneLine) add("E2", "the Done command exits 0 on a clean git tree", false, ["not run: H6 fails, so there is no Done command to run"]);
  else {
    const dirty = git(root, ["status", "--porcelain"]);
    if (dirty === null) add("E2", "the Done command exits 0 on a clean git tree", false, ["not a git repository: commit your own paths first"]);
    else if (dirty.trim() !== "") {
      const files = dirty.split("\n").filter(Boolean).slice(0, 10);
      add("E2", "the Done command exits 0 on a clean git tree", false, ["the git tree is not clean: commit your own paths first", ...files]);
    } else {
      const command = DONE_RE.exec(doneLine.l)[1].trim();
      const r = await runCommand(root, command, seconds);
      executed.verify = { command: r.command, exit: r.exit, seconds: r.seconds, treeClean: true, tail: r.tail };
      add("E2", "the Done command exits 0 on a clean git tree", r.exit === 0, describeRun("Done", r));
    }
  }
  report.executed = executed;
  const finished = summarize(ctx, report.checks, { quick: false, cert: true, run: true, accept });
  Object.assign(report, finished, { executed });
  return report;
}

/** Writes the certificate. Only a run that certified (every gate and cert check passed or was accepted, E1 and E2 ran) may write it. */
function writeCert(root, report, args) {
  const sha = git(root, ["rev-parse", "HEAD"])?.trim();
  if (!report.certified || !sha) return { written: false, reason: !sha ? "not a git repository" : `not certified: ${report.blocking.join(", ")}` };
  const passed = (level) => report.checks.filter((c) => c.level === level && c.status === "pass").map((c) => c.id);
  const cfg = readJson(root, "docs/sdlc/harness.json") ?? {};
  const cert = {
    schema: 1,
    date: new Date().toISOString().slice(0, 10),
    sha,
    profile: Array.isArray(cfg.profile) ? cfg.profile.filter((p) => typeof p === "string") : [],
    checks: { gate: passed("gate"), cert: passed("cert"), warnings: report.warnings },
    executed: {
      ensure: report.executed?.ensure ? { command: report.executed.ensure.command, exit: report.executed.ensure.exit, seconds: report.executed.ensure.seconds } : null,
      verify: report.executed?.verify
        ? { command: report.executed.verify.command, exit: report.executed.verify.exit, seconds: report.executed.verify.seconds, treeClean: report.executed.verify.treeClean }
        : null,
    },
    footprint: report.footprint,
    accepted: args.accept.filter((a) => report.accepted.includes(a.check)).map((a) => a.check),
    assumptions: [...new Set(args.accept.filter((a) => report.accepted.includes(a.check)).map((a) => a.assumption))],
  };
  const target = join(root, CERT_PATH);
  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, `${JSON.stringify(cert, null, 2)}\n`);
  return { written: true, path: CERT_PATH };
}

async function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
    if (!existsSync(args.root) || !statSync(args.root).isDirectory()) throw new Error(`not a directory: ${args.root}`);
  } catch (err) {
    process.stderr.write(`check-harness: ${err.message}\nusage: check-harness.mjs [--root <dir>] [--quick] [--cert] [--run [--timeout <s>]] [--refresh]\n`);
    process.exit(2);
  }
  let report = checkHarness(args.root, args);
  if (args.run) report = await runExecutedChecks(args.root, report, { timeout: args.timeout, accept: args.accept });
  if (args.writeCert) {
    const res = writeCert(args.root, report, args);
    report.certificate = res;
    if (!res.written) process.stderr.write(`check-harness: certificate not written (${res.reason})\n`);
  }
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  process.exit(report.blocking.length === 0 ? 0 : 1);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
