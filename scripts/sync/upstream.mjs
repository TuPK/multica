#!/usr/bin/env node
// `pnpm sync:upstream`: one command for an upstream sync. It never commits or pushes.
//
//   pnpm sync:upstream               start: preflight, branch, merge, policy cleanup, rebrand
//   pnpm sync:upstream --finish      after you resolved conflicts: regenerate, checks, audit
//   options: --upstream <ref> (default upstream/main)  --no-fetch  --skip-regenerate (with --finish)
//
// Steps and reasons: docs/specs/upstream-sync/spec.md section 3.5.
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { parseArgs } from "node:util";
import { fileURLToPath, pathToFileURL } from "node:url";
import { check as doctorCheck } from "./doctor.mjs";
import { countByClass } from "./classify.mjs";
import { rebrandUpstreamOnly } from "./rebrand.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const git = (root, ...args) => execFileSync("git", args, { cwd: root, encoding: "utf8", maxBuffer: 1 << 30, stdio: ["ignore", "pipe", "pipe"] });
const statePath = (root) => join(git(root, "rev-parse", "--absolute-git-dir").trim(), "sync-state.json");
const loadPolicy = (file = join(here, "policy.json")) => JSON.parse(readFileSync(file, "utf8"));

/** Changed files that still contain git conflict markers (resolved by `git add` alone). */
export function markerFiles(root, base = "HEAD") {
  const changed = git(root, "diff", "--name-only", "--diff-filter=AM", base).split("\n").filter(Boolean);
  return changed.filter((f) => {
    try {
      return /^(<{7}|>{7}) /m.test(readFileSync(join(root, f), "utf8"));
    } catch {
      return false;
    }
  });
}

export function unmerged(root) {
  return git(root, "diff", "--name-only", "--diff-filter=U").split("\n").filter(Boolean);
}

/** Labels each remaining conflict with its class and, when known, the seam that removes it. */
export function describeConflicts(files, policy) {
  return files.map((f) => ({ file: f, seam: policy.hotFiles?.[f] ?? null }));
}

export function start({ root, upstream = "upstream/main", fetch = true, skipDoctor = false, policy = loadPolicy(), date = new Date().toISOString().slice(0, 10) }) {
  const log = [];
  if (git(root, "status", "--porcelain").trim()) throw new Error("working tree is not clean; commit or stash first");
  const branch = git(root, "rev-parse", "--abbrev-ref", "HEAD").trim();
  if (branch !== "main") throw new Error(`start from main (currently on ${branch})`);
  if (!skipDoctor) {
    const problems = doctorCheck();
    if (problems.length) throw new Error(`sync:doctor failed:\n  ${problems.join("\n  ")}`);
  }
  if (fetch) git(root, "fetch", upstream.split("/")[0]);
  const ahead = Number(git(root, "rev-list", "--count", `HEAD..${upstream}`).trim());
  if (!ahead) throw new Error(`nothing to merge: ${upstream} has no commits HEAD lacks`);
  const pre = git(root, "rev-parse", "HEAD").trim();
  const up = git(root, "rev-parse", upstream).trim();
  const base = git(root, "merge-base", pre, up).trim();
  let syncBranch = `chore/sync-upstream-${date}`;
  for (let n = 2; git(root, "branch", "--list", syncBranch).trim(); n++) syncBranch = `chore/sync-upstream-${date}-${n}`;
  git(root, "checkout", "-b", syncBranch);
  log.push(`branch ${syncBranch}; merging ${ahead} upstream commits (base ${base.slice(0, 9)})`);
  try {
    git(root, "merge", "--no-commit", "--no-ff", upstream);
  } catch {
    /* conflicts are expected */
  }
  for (const p of policy.excluded ?? []) {
    if (git(root, "ls-files", "--", p).trim() || existsSync(join(root, p))) {
      git(root, "rm", "-rq", "--ignore-unmatch", "--", p);
      log.push(`excluded by policy: removed ${p}`);
    }
  }
  const rewritten = rebrandUpstreamOnly({ root, base, fork: pre, upstream: up });
  if (rewritten.length) log.push(`rebranded upstream-added lines in ${rewritten.length} files`);
  writeFileSync(statePath(root), JSON.stringify({ pre, upstream: up, base, branch: syncBranch, ahead, date }, null, 2));
  const conflicts = describeConflicts(unmerged(root), policy);
  return { log, conflicts, byClass: countByClass(conflicts.map((c) => c.file)), ahead };
}

function sh(root, command) {
  const r = spawnSync(command, { cwd: root, shell: true, stdio: "inherit" });
  return r.status === 0;
}

export function finish({ root, policy = loadPolicy(), run = sh, skipRegenerate = false }) {
  if (!existsSync(statePath(root))) throw new Error("no sync in progress (run `pnpm sync:upstream` first)");
  const state = JSON.parse(readFileSync(statePath(root), "utf8"));
  const left = unmerged(root);
  if (left.length) throw new Error(`${left.length} files are still unmerged, e.g. ${left.slice(0, 3).join(", ")}`);
  const steps = [];
  const step = (name, ok, note = "") => steps.push({ name, ok, note });
  const markers = markerFiles(root);
  step("no conflict markers left in changed files", markers.length === 0, markers.slice(0, 5).join(", "));
  if (markers.length) return { steps, ok: false, state };
  if (!skipRegenerate) {
    for (const g of [...policy.regenerate].sort((a, b) => a.order - b.order)) step(`regenerate ${g.name}`, run(root, g.command), g.command);
  }
  step("brand:check", run(root, "node scripts/sync/brand-check.mjs"));
  const worklist = join(tmpdir(), "vi-worklist.json");
  const vi = run(root, `node scripts/sync/vi-worklist.mjs --list --out ${worklist}`);
  step("vi locale covers every English key", vi, vi ? "" : `translate ${worklist}, then node scripts/sync/vi-worklist.mjs --apply ${worklist}`);
  step("sync-audit (review every hit)", run(root, `node scripts/sync-audit.mjs --fork ${state.pre} --upstream ${state.upstream}`));
  step("sync:contract", run(root, "node scripts/sync/contract.mjs"));
  return { steps, ok: steps.every((s) => s.ok), state };
}

export const CHECKLIST = [
  "pnpm typecheck && pnpm lint",
  "pnpm test (serial on a loaded machine: turbo --concurrency=1)",
  "make test (go test -p 1 when the machine is loaded; DB tests use multica_test only)",
  "pnpm test:scripts",
  "check new migrations: concurrent-index ones registered (fork ones in server/cmd/migrate/fork_index_cleanups.go), and a fresh install works: scratch DB + `cd server && DATABASE_URL=<scratch> go run ./cmd/migrate up`, then drop it",
  "git add -A && git commit, push the branch, fast-forward main (docs/fork-branding.md, ledger.md)",
];

export function main(argv) {
  const { values: v } = parseArgs({ args: argv, options: { finish: { type: "boolean" }, upstream: { type: "string", default: "upstream/main" }, "skip-regenerate": { type: "boolean" }, "no-fetch": { type: "boolean" } } });
  const root = execFileSync("git", ["rev-parse", "--show-toplevel"], { encoding: "utf8" }).trim();
  try {
    if (v.finish) {
      const r = finish({ root, skipRegenerate: v["skip-regenerate"] });
      console.log("\n== sync:upstream --finish");
      for (const s of r.steps) console.log(`${s.ok ? "ok  " : "FAIL"} ${s.name}${!s.ok && s.note ? `  (${s.note})` : ""}`);
      if (r.ok) console.log(`\nNext:\n${CHECKLIST.map((c) => `  - ${c}`).join("\n")}`);
      return r.ok ? 0 : 1;
    }
    const r = start({ root, upstream: v.upstream, fetch: !v["no-fetch"] });
    console.log(r.log.join("\n"));
    if (!r.conflicts.length) {
      console.log("no conflicts left; run `pnpm sync:upstream --finish`");
      return 0;
    }
    console.log(`\n${r.conflicts.length} conflicts to resolve by hand:`);
    for (const [k, n] of Object.entries(r.byClass).sort((a, b) => b[1] - a[1])) console.log(`  ${String(n).padStart(4)}  ${k}`);
    for (const c of r.conflicts) console.log(`  ${c.file}${c.seam ? `   [recurring: seam ${c.seam}]` : ""}`);
    console.log("\nResolve, `git add` them, then run `pnpm sync:upstream --finish`.");
    return 0;
  } catch (e) {
    console.error(`sync:upstream: ${e.message}`);
    return 1;
  }
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
