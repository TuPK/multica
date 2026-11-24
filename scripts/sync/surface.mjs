#!/usr/bin/env node
// `pnpm sync:surface`: how far the fork has drifted from upstream, as a number you can
// watch. Counts files the fork added, modified and deleted relative to upstream, and how
// many of the modified ones differ only in brand spelling.
//
//   node scripts/sync/surface.mjs                       report
//   node scripts/sync/surface.mjs --write               store docs/specs/upstream-sync/surface-baseline.json
//   node scripts/sync/surface.mjs --check               exit 1 when modified files grew past the baseline
//   options: --upstream <ref> (default upstream/main)  --head <ref> (default HEAD)
//
// The number that matters is "modified": each such file is a place a future upstream
// change can conflict. See docs/specs/upstream-sync/spec.md section 3.8.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { parseArgs } from "node:util";
import { pathToFileURL } from "node:url";
import { canon } from "./brand-rules.mjs";

const BASELINE = "docs/specs/upstream-sync/surface-baseline.json";

/** Parses `git diff -U0` into per-file removed and added line lists. */
export function parseDiff(text) {
  const files = new Map();
  let cur = null;
  for (const line of text.split("\n")) {
    if (line.startsWith("diff --git ")) {
      const m = /^diff --git a\/(.+) b\/(.+)$/.exec(line);
      cur = { removed: [], added: [] };
      files.set(m ? m[2] : line, cur);
    } else if (!cur || line.startsWith("+++") || line.startsWith("---")) continue;
    else if (line.startsWith("+")) cur.added.push(line.slice(1));
    else if (line.startsWith("-")) cur.removed.push(line.slice(1));
  }
  return files;
}

/** True when the two sides differ only in Delegora/Multica spelling. */
export function brandOnly({ removed, added }) {
  if (removed.length !== added.length) return false;
  const a = removed.map(canon).sort();
  const b = added.map(canon).sort();
  return a.every((x, i) => x === b[i]);
}

export function measure({ upstream, head, git }) {
  const status = git("diff", "--name-status", "--no-renames", upstream, head)
    .split("\n")
    .filter(Boolean)
    .map((l) => l.split("\t"));
  const counts = { added: 0, modified: 0, deleted: 0 };
  const modified = [];
  for (const [s, f] of status) {
    if (s === "A") counts.added += 1;
    else if (s === "D") counts.deleted += 1;
    else if (s === "M") {
      counts.modified += 1;
      modified.push(f);
    }
  }
  const diffs = parseDiff(git("diff", "-U0", "--no-renames", "--diff-filter=M", upstream, head));
  let onlyBrand = 0;
  const byDir = {};
  for (const f of modified) {
    const d = diffs.get(f);
    if (d && brandOnly(d)) onlyBrand += 1;
    else {
      const key = f.split("/").slice(0, 3).join("/");
      byDir[key] = (byDir[key] ?? 0) + 1;
    }
  }
  const top = Object.entries(byDir).sort((a, b) => b[1] - a[1]).slice(0, 25);
  return { ...counts, brandOnly: onlyBrand, substantive: counts.modified - onlyBrand, top };
}

export function main(argv) {
  const { values: v } = parseArgs({
    args: argv,
    options: {
      upstream: { type: "string", default: "upstream/main" },
      head: { type: "string", default: "HEAD" },
      write: { type: "boolean", default: false },
      check: { type: "boolean", default: false },
    },
  });
  const git = (...args) => execFileSync("git", args, { encoding: "utf8", maxBuffer: 1 << 30 });
  const root = git("rev-parse", "--show-toplevel").trim();
  const m = measure({ upstream: v.upstream, head: v.head, git: (...a) => execFileSync("git", a, { cwd: root, encoding: "utf8", maxBuffer: 1 << 30 }) });
  const stamp = { upstream: git("rev-parse", v.upstream).trim(), head: git("rev-parse", v.head).trim() };
  console.log(`fork vs ${v.upstream}: added ${m.added}, modified ${m.modified} (brand-only ${m.brandOnly}, substantive ${m.substantive}), deleted ${m.deleted}`);
  for (const [k, n] of m.top.slice(0, 12)) console.log(`  ${String(n).padStart(4)}  ${k}`);
  if (v.write) {
    writeFileSync(`${root}/${BASELINE}`, JSON.stringify({ ...stamp, added: m.added, modified: m.modified, brandOnly: m.brandOnly, substantive: m.substantive, deleted: m.deleted }, null, 2) + "\n");
    console.log(`wrote ${BASELINE}`);
  }
  if (v.check) {
    const base = JSON.parse(readFileSync(`${root}/${BASELINE}`, "utf8"));
    if (m.substantive > base.substantive) {
      console.error(`surface grew: substantive modified files ${base.substantive} -> ${m.substantive}. Put new fork code in fork-owned files (spec 3.9) or refresh the baseline with a reason.`);
      return 1;
    }
    console.log(`surface ok: substantive ${m.substantive} <= baseline ${base.substantive}`);
  }
  return 0;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
