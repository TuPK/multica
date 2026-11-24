#!/usr/bin/env node
// `pnpm sync:simulate`: dry-runs a merge in a throwaway worktree and reports which files
// would conflict. Nothing in your checkout changes.
//
//   node scripts/sync/simulate.mjs --merge <commit>        replay a past merge (its two parents)
//   node scripts/sync/simulate.mjs --fork <ref> --upstream <ref>
//   options: --drivers on|off (default on)   --compare   --json
//
// --compare also checks every file that merged cleanly against what the real merge commit
// (`--merge`) contains, ignoring Delegora/Multica spelling and JSON key order, and lists
// the files that differ. See docs/specs/upstream-sync/spec.md sections 2.1, 2.4 and 3.10.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseArgs } from "node:util";
import { pathToFileURL } from "node:url";
import { canon } from "./brand-rules.mjs";
import { countByClass } from "./classify.mjs";

const here = new URL(".", import.meta.url).pathname;

function sortKeys(v) {
  if (Array.isArray(v)) return v.map(sortKeys);
  if (v && typeof v === "object") return Object.fromEntries(Object.keys(v).sort().map((k) => [k, sortKeys(v[k])]));
  return v;
}

/** Normal form used to compare a driver result with the real merge's file. */
export function normalizeForCompare(path, text) {
  if (path.endsWith(".json")) {
    try {
      return canon(JSON.stringify(sortKeys(JSON.parse(text))));
    } catch {
      /* fall through to text */
    }
  }
  return canon(text);
}

export function simulate({ root, fork, upstream, drivers = true, compareWith = null }) {
  const git = (cwd, ...args) => execFileSync("git", args, { cwd, encoding: "utf8", maxBuffer: 1 << 30, stdio: ["ignore", "pipe", "pipe"] });
  const tmp = mkdtempSync(join(tmpdir(), "sync-sim-"));
  const wt = join(tmp, "wt");
  const flags = ["-c", "rerere.enabled=false"];
  try {
    if (drivers) {
      const attrs = join(tmp, "attrs");
      writeFileSync(attrs, readFileSync(join(root, ".gitattributes"), "utf8"));
      flags.push(
        "-c", `core.attributesFile=${attrs}`,
        "-c", `merge.brand.driver=node ${here}merge-brand.mjs %O %A %B %L %P`,
        "-c", `merge.jsonmerge.driver=node ${here}merge-json.mjs %O %A %B %L`,
        "-c", "merge.ours.driver=true",
      );
    }
    if (!drivers) {
      // Neutralise any driver this checkout defines: plain three-way text merge.
      const plain = "git merge-file --marker-size=%L %A %O %B";
      for (const name of ["brand", "jsonmerge", "ours"]) flags.push("-c", `merge.${name}.driver=${plain}`);
    }
    git(root, "worktree", "add", "-q", "--detach", wt, fork);
    try {
      git(wt, ...flags, "merge", "--no-commit", "--no-ff", upstream);
    } catch {
      /* conflicts are the point */
    }
    const conflicts = git(wt, "diff", "--name-only", "--diff-filter=U").split("\n").filter(Boolean);
    const result = { conflicts, byClass: countByClass(conflicts), compared: null };
    if (compareWith) {
      const unmerged = new Set(conflicts);
      const changed = git(wt, "diff", "--cached", "--name-only", "HEAD").split("\n").filter(Boolean);
      const differ = [];
      let checked = 0;
      for (const f of changed) {
        if (unmerged.has(f)) continue;
        let real;
        try {
          real = git(root, "show", `${compareWith}:${f}`);
        } catch {
          differ.push({ file: f, why: "absent in the real merge" });
          continue;
        }
        let mine;
        try {
          mine = readFileSync(join(wt, f), "utf8");
        } catch {
          continue;
        }
        checked += 1;
        if (normalizeForCompare(f, mine) !== normalizeForCompare(f, real)) differ.push({ file: f, why: "content differs" });
      }
      result.compared = { checked, differ };
    }
    try {
      git(wt, "merge", "--abort");
    } catch {
      /* nothing to abort */
    }
    return result;
  } finally {
    try {
      git(root, "worktree", "remove", "--force", wt);
    } catch {
      /* already gone */
    }
    rmSync(tmp, { recursive: true, force: true });
  }
}

export function main(argv) {
  const { values: v } = parseArgs({
    args: argv,
    options: {
      merge: { type: "string" },
      fork: { type: "string" },
      upstream: { type: "string" },
      drivers: { type: "string", default: "on" },
      compare: { type: "boolean", default: false },
      json: { type: "boolean", default: false },
    },
  });
  const root = execFileSync("git", ["rev-parse", "--show-toplevel"], { encoding: "utf8" }).trim();
  const rev = (r) => execFileSync("git", ["rev-parse", r], { cwd: root, encoding: "utf8" }).trim();
  let fork = v.fork;
  let upstream = v.upstream;
  if (v.merge) {
    fork = rev(`${v.merge}^1`);
    upstream = rev(`${v.merge}^2`);
  }
  if (!fork || !upstream) {
    console.error("usage: sync-simulate --merge <commit> | --fork <ref> --upstream <ref> [--drivers on|off] [--compare] [--json]");
    return 2;
  }
  if (v.compare && !v.merge) {
    console.error("--compare needs --merge <commit> (the real merge to compare with)");
    return 2;
  }
  const res = simulate({ root, fork, upstream, drivers: v.drivers !== "off", compareWith: v.compare ? v.merge : null });
  if (v.json) {
    console.log(JSON.stringify(res, null, 2));
    return 0;
  }
  console.log(`drivers=${v.drivers}  fork=${fork.slice(0, 9)}  upstream=${upstream.slice(0, 9)}`);
  console.log(`conflicts: ${res.conflicts.length}`);
  for (const [k, n] of Object.entries(res.byClass).sort((a, b) => b[1] - a[1])) console.log(`  ${String(n).padStart(4)}  ${k}`);
  if (res.compared) {
    console.log(`compared ${res.compared.checked} clean files with ${v.merge}: ${res.compared.differ.length} differ`);
    for (const d of res.compared.differ.slice(0, 40)) console.log(`  ${d.file}  (${d.why})`);
  }
  return 0;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
