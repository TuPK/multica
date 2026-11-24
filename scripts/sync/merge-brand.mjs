#!/usr/bin/env node
// Git merge driver for text files: `node merge-brand.mjs %O %A %B %L %P`.
//
// Merges in the upstream spelling (Delegora -> Multica on all three sides) so brand
// edits never conflict, then maps every result line back: a line the fork already had
// keeps the fork's exact bytes (including a deliberate `Multica`), and a line that came
// from upstream gets brand(). Anything uncertain exits 1 and leaves git's own conflict
// markers in %A. See docs/specs/upstream-sync/spec.md section 3.1.
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { brand, canon } from "./brand-rules.mjs";

/**
 * Pure core: three texts in, `{ text, conflict }` out. `text` carries conflict markers
 * when `conflict` is true. `mergeFile(ours, base, theirs)` must behave like
 * `git merge-file -p` and return `{ text, conflict }`.
 */
export function mergeBranded({ base, ours, theirs }, mergeFile) {
  const merged = mergeFile(canon(ours), canon(base), canon(theirs));
  // Fork lines by their upstream spelling, consumed in order so repeated lines map 1:1.
  const pool = new Map();
  for (const line of ours.split("\n")) {
    const key = canon(line);
    if (!pool.has(key)) pool.set(key, []);
    pool.get(key).push(line);
  }
  const text = merged.text
    .split("\n")
    .map((line) => {
      const queue = pool.get(line);
      return queue?.length ? queue.shift() : brand(line);
    })
    .join("\n");
  return { text, conflict: merged.conflict };
}

export function gitMergeFile(ours, base, theirs, markerSize = 7) {
  const dir = mkdtempSync(join(tmpdir(), "merge-brand-"));
  try {
    const [a, o, b] = ["a", "o", "b"].map((n) => join(dir, n));
    writeFileSync(a, ours);
    writeFileSync(o, base);
    writeFileSync(b, theirs);
    const r = spawnSync(
      "git",
      ["merge-file", "-p", "-L", "ours", "-L", "base", "-L", "theirs", `--marker-size=${markerSize}`, a, o, b],
      { encoding: "utf8", maxBuffer: 1 << 30 },
    );
    if (r.status === null || r.status < 0 || r.status > 127) throw new Error(r.stderr || "git merge-file failed");
    return { text: r.stdout, conflict: r.status > 0 };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

export function main(argv) {
  const [O, A, B, L] = argv;
  const [base, ours, theirs] = [O, A, B].map((f) => readFileSync(f, "utf8"));
  if ([base, ours, theirs].some((t) => t.includes("\0"))) return 1;
  const { text, conflict } = mergeBranded({ base, ours, theirs }, (a, o, b) => gitMergeFile(a, o, b, Number(L) || 7));
  writeFileSync(A, text);
  return conflict ? 1 : 0;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
