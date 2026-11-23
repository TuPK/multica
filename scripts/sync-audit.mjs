#!/usr/bin/env node
// Post-merge audit for an upstream sync: finds what the fork lost.
//
//   node scripts/sync-audit.mjs --fork <pre-merge commit> [--upstream upstream/main]
//                               [--base <merge-base>] [--head HEAD] [--min-len 12] [--top 60]
//
// Defaults: --base is `git merge-base <fork> <upstream>`, --head is HEAD.
// Reports (1) lines the fork had that are gone from head and that upstream did not
// remove itself, per file, and (2) files the fork added (absent from base) and
// head no longer has. Review each hit: a hit is a candidate, not proof of loss.
// Locale JSON, lockfiles and generated code are skipped; add them back with --all.
import { execFileSync } from "node:child_process";
import { parseArgs } from "node:util";

const { values: opt } = parseArgs({
  options: {
    fork: { type: "string" },
    upstream: { type: "string", default: "upstream/main" },
    base: { type: "string" },
    head: { type: "string", default: "HEAD" },
    "min-len": { type: "string", default: "12" },
    top: { type: "string", default: "60" },
    all: { type: "boolean", default: false },
  },
});
if (!opt.fork) {
  console.error("usage: node scripts/sync-audit.mjs --fork <pre-merge commit> [--upstream ref] [--base commit] [--head ref]");
  process.exit(2);
}
const git = (...args) =>
  execFileSync("git", args, { encoding: "utf8", maxBuffer: 1 << 30 });
const base = opt.base ?? git("merge-base", opt.fork, opt.upstream).trim();
const minLen = Number(opt["min-len"]);
const SKIP = /(\.(json|lock|snap|sum)$|\/locales\/|\/generated\/|pnpm-lock\.yaml$)/;

function removedLines(from, to) {
  const byFile = new Map();
  let file = null;
  for (const line of git("diff", "-U0", "--no-renames", from, to).split("\n")) {
    if (line.startsWith("--- a/")) file = line.slice(6);
    else if (line.startsWith("-") && !line.startsWith("---") && file) {
      if (!byFile.has(file)) byFile.set(file, new Set());
      byFile.get(file).add(line.slice(1).trim());
    }
  }
  return byFile;
}

const ours = removedLines(opt.fork, opt.head);
const theirs = removedLines(base, opt.upstream);
const hits = [];
for (const [file, lines] of ours) {
  if (!opt.all && SKIP.test(file)) continue;
  const lost = [...lines].filter((l) => l.length >= minLen && !theirs.get(file)?.has(l));
  if (lost.length) hits.push({ file, lost });
}
hits.sort((a, b) => b.lost.length - a.lost.length);

console.log(`fork=${opt.fork} base=${base} upstream=${opt.upstream} head=${opt.head}\n`);
console.log(`== Fork lines gone from head and not removed by upstream (${hits.length} files)`);
for (const { file, lost } of hits.slice(0, Number(opt.top))) {
  console.log(`${String(lost.length).padStart(4)}  ${file}`);
  for (const l of lost.slice(0, 5)) console.log(`        - ${l.slice(0, 140)}`);
}

const exists = (ref, file) => {
  try {
    git("cat-file", "-e", `${ref}:${file}`);
    return true;
  } catch {
    return false;
  }
};
const deleted = git("diff", "--name-status", "--no-renames", opt.fork, opt.head)
  .split("\n")
  .filter((l) => l.startsWith("D\t"))
  .map((l) => l.slice(2))
  .filter((f) => !exists(base, f));
console.log(`\n== Files the fork added that head no longer has (${deleted.length})`);
for (const f of deleted) console.log(`  ${f}`);
