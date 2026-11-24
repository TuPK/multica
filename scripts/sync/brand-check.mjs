#!/usr/bin/env node
// `pnpm brand:check`: reports a bare `Multica` on surfaces people read (locales, landing
// copy, READMEs, docs). Existing leftovers live in scripts/sync/brand-debt.json; the check
// fails only when a file has more of them than the recorded debt, so new upstream text
// that was not rebranded cannot slip in.
//
//   node scripts/sync/brand-check.mjs                 check against the debt file
//   node scripts/sync/brand-check.mjs --write-debt    record the current state as the debt
//   node scripts/sync/brand-check.mjs --list          print every hit
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { parseArgs } from "node:util";
import { pathToFileURL } from "node:url";
import { bareUpstreamName, isDisplaySurface } from "./brand-rules.mjs";

const DEBT = "scripts/sync/brand-debt.json";

export function scan({ root, files }) {
  const hits = {};
  for (const f of files.filter(isDisplaySurface)) {
    let text;
    try {
      text = readFileSync(`${root}/${f}`, "utf8");
    } catch {
      continue;
    }
    const h = bareUpstreamName(text);
    if (h.length) hits[f] = h;
  }
  return hits;
}

/** Files whose hit count exceeds the recorded debt. */
export function newHits(hits, debt) {
  return Object.entries(hits)
    .filter(([f, h]) => h.length > (debt[f] ?? 0))
    .map(([f, h]) => ({ file: f, found: h.length, allowed: debt[f] ?? 0, sample: h.slice(debt[f] ?? 0, (debt[f] ?? 0) + 3) }));
}

export function main(argv) {
  const { values: v } = parseArgs({ args: argv, options: { "write-debt": { type: "boolean" }, list: { type: "boolean" } } });
  const root = execFileSync("git", ["rev-parse", "--show-toplevel"], { encoding: "utf8" }).trim();
  const files = execFileSync("git", ["ls-files"], { cwd: root, encoding: "utf8", maxBuffer: 1 << 28 }).split("\n").filter(Boolean);
  const hits = scan({ root, files });
  if (v.list) {
    for (const [f, h] of Object.entries(hits)) for (const x of h) console.log(`${f}:${x.line}: ${x.text}`);
    return 0;
  }
  if (v["write-debt"]) {
    const debt = Object.fromEntries(Object.entries(hits).map(([f, h]) => [f, h.length]).sort());
    writeFileSync(`${root}/${DEBT}`, JSON.stringify(debt, null, 2) + "\n");
    console.log(`recorded ${Object.keys(debt).length} files with leftover Multica in ${DEBT}`);
    return 0;
  }
  let debt = {};
  try {
    debt = JSON.parse(readFileSync(`${root}/${DEBT}`, "utf8"));
  } catch {
    /* no debt file: everything counts */
  }
  const bad = newHits(hits, debt);
  if (!bad.length) {
    console.log(`brand:check ok (${Object.keys(hits).length} files carry recorded leftovers)`);
    return 0;
  }
  for (const b of bad) {
    console.error(`${b.file}: ${b.found} bare "Multica" (allowed ${b.allowed})`);
    for (const s of b.sample) console.error(`  ${b.file}:${s.line}: ${s.text}`);
  }
  console.error("Display text says Delegora (docs/fork-branding.md). Fix the lines, or if Multica is intended add an exception to scripts/sync/brand-rules.mjs.");
  return 1;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
