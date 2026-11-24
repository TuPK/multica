#!/usr/bin/env node
// Git merge driver for JSON data files (locales, docs meta): `node merge-json.mjs %O %A %B %L`.
//
// Merges by key path instead of by line, so two sides adding or removing neighbouring
// keys no longer conflict. Brand differences (Delegora vs Multica) are ignored when
// comparing, and strings that come from upstream are branded. Anything it cannot decide
// falls back to the brand-aware line merge, so a real conflict still carries markers. See
// docs/specs/upstream-sync/spec.md section 3.2.
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { brand, canon } from "./brand-rules.mjs";
import { gitMergeFile as gitMergeBranded, mergeBranded } from "./merge-brand.mjs";

export class Unmergeable extends Error {}

const ABSENT = Symbol("absent");
const isObject = (v) => v !== null && typeof v === "object" && !Array.isArray(v);
const isScalar = (v) => v === null || ["string", "number", "boolean"].includes(typeof v);
const isScalarArray = (v) => Array.isArray(v) && v.every(isScalar);
const ser = (v) => (v === ABSENT ? "\0absent" : canon(JSON.stringify(v)));
const same = (x, y) => ser(x) === ser(y);

function brandDeep(v) {
  if (typeof v === "string") return brand(v);
  if (Array.isArray(v)) return v.map(brandDeep);
  if (isObject(v)) return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, brandDeep(x)]));
  return v;
}

/** Line merge of scalar arrays, one element per line, in the upstream spelling. */
function mergeScalarArrays(o, a, b, mergeLines) {
  const lines = (arr) => arr.map((x) => canon(JSON.stringify(x))).join("\n") + (arr.length ? "\n" : "");
  const merged = mergeLines(lines(a), lines(o), lines(b));
  if (merged.conflict) throw new Unmergeable("array conflict");
  const pool = new Map();
  for (const x of a) {
    const k = canon(JSON.stringify(x));
    if (!pool.has(k)) pool.set(k, []);
    pool.get(k).push(x);
  }
  return merged.text
    .split("\n")
    .filter((l) => l !== "")
    .map((l) => {
      const queue = pool.get(l);
      return queue?.length ? queue.shift() : brandDeep(JSON.parse(l));
    });
}

function merge(o, a, b, mergeLines) {
  if (same(a, b)) return a;
  if (same(o, a)) return b === ABSENT ? ABSENT : brandDeep(b);
  if (same(o, b)) return a;
  if (isObject(a) && isObject(b) && (isObject(o) || o === ABSENT)) {
    const base = o === ABSENT ? {} : o;
    const keys = Object.keys(a);
    const theirKeys = Object.keys(b);
    theirKeys.forEach((k, i) => {
      if (keys.includes(k)) return;
      const prev = theirKeys.slice(0, i).reverse().find((p) => keys.includes(p));
      keys.splice(prev === undefined ? 0 : keys.indexOf(prev) + 1, 0, k);
    });
    const out = {};
    for (const k of keys) {
      const r = merge(k in base ? base[k] : ABSENT, k in a ? a[k] : ABSENT, k in b ? b[k] : ABSENT, mergeLines);
      if (r !== ABSENT) out[k] = r;
    }
    return out;
  }
  if (isScalarArray(a) && isScalarArray(b) && isScalarArray(o)) return mergeScalarArrays(o, a, b, mergeLines);
  throw new Unmergeable("both sides changed the same value");
}

const format = (value, finalNewline) => JSON.stringify(value, null, 2) + (finalNewline ? "\n" : "");

/**
 * Pure core. Returns the merged file text, or throws Unmergeable. `mergeLines(ours, base,
 * theirs)` behaves like `git merge-file -p` and returns `{ text, conflict }`.
 */
export function mergeJson({ base, ours, theirs }, mergeLines) {
  const parse = (t) => {
    try {
      return JSON.parse(t);
    } catch {
      throw new Unmergeable("invalid JSON");
    }
  };
  const [o, a, b] = [base, ours, theirs].map(parse);
  const finalNewline = ours.endsWith("\n");
  // Only rewrite files whose layout survives parse + stringify unchanged.
  if (format(a, finalNewline) !== ours) throw new Unmergeable("fork file is not in canonical JSON layout");
  return format(merge(o, a, b, mergeLines), finalNewline);
}

function gitMergeFile(ours, base, theirs) {
  const dir = mkdtempSync(join(tmpdir(), "merge-json-"));
  try {
    const [a, o, b] = ["a", "o", "b"].map((n) => join(dir, n));
    writeFileSync(a, ours);
    writeFileSync(o, base);
    writeFileSync(b, theirs);
    const r = spawnSync("git", ["merge-file", "-p", a, o, b], { encoding: "utf8" });
    if (r.status === null || r.status < 0 || r.status > 127) throw new Error(r.stderr || "git merge-file failed");
    return { text: r.stdout, conflict: r.status > 0 };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

export function main([O, A, B, L]) {
  const input = { base: readFileSync(O, "utf8"), ours: readFileSync(A, "utf8"), theirs: readFileSync(B, "utf8") };
  try {
    const text = mergeJson(input, gitMergeFile);
    writeFileSync(A, text);
    return 0;
  } catch (e) {
    if (!(e instanceof Unmergeable)) console.error(e);
    // Fall back to the brand-aware line merge so git still gets conflict markers in the file.
    const { text, conflict } = mergeBranded(input, (a, o, b) => gitMergeBranded(a, o, b, Number(L) || 7));
    writeFileSync(A, text);
    return conflict ? 1 : 0;
  }
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
