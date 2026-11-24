#!/usr/bin/env node
// `pnpm sync:vi`: the `vi` locale exists only in this fork and must cover every English
// key (locales/parity.test.ts). After a sync, upstream's new English keys leave `vi`
// short. This lists the missing keys and writes translations back; it translates nothing.
//
//   node scripts/sync/vi-worklist.mjs --list [--out worklist.json]   missing keys: [{file, path, en}]
//   node scripts/sync/vi-worklist.mjs --apply worklist.json          fill each entry's "vi" and drop extras with --prune
//   node scripts/sync/vi-worklist.mjs --extra                        keys `vi` has that English no longer has
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { parseArgs } from "node:util";
import { pathToFileURL } from "node:url";

const isObj = (v) => v !== null && typeof v === "object" && !Array.isArray(v);

// Vietnamese has one plural form, so English `_one` (and `_zero`, `_two`, ...) keys are not
// required in `vi`; only `_other` is. Same rule i18next applies when it resolves a key.
const VI_FORMS = new Set(new Intl.PluralRules("vi").resolvedOptions().pluralCategories);
const PLURAL = /_(zero|one|two|few|many|other)$/;
const notNeeded = (key) => {
  const m = PLURAL.exec(key);
  return Boolean(m) && !VI_FORMS.has(m[1]);
};

export function missing(en, vi, prefix = []) {
  const out = [];
  for (const [k, v] of Object.entries(en)) {
    const path = [...prefix, k];
    if (isObj(v)) out.push(...missing(v, isObj(vi?.[k]) ? vi[k] : {}, path));
    else if (!(isObj(vi) && k in vi) && !notNeeded(k)) out.push({ path, en: v });
  }
  return out;
}

export function extra(en, vi, prefix = []) {
  const out = [];
  for (const [k, v] of Object.entries(vi ?? {})) {
    const path = [...prefix, k];
    if (!(isObj(en) && k in en)) out.push(path);
    else if (isObj(v) && isObj(en[k])) out.push(...extra(en[k], v, path));
  }
  return out;
}

/** Puts `value` at `path`, creating objects on the way. */
function setPath(obj, path, value) {
  let cur = obj;
  for (const k of path.slice(0, -1)) cur = isObj(cur[k]) ? cur[k] : (cur[k] = {});
  cur[path.at(-1)] = value;
}

/** Rebuilds `vi` in the key order of `en`; keys English lacks stay at the end. */
export function reorder(en, vi) {
  const out = {};
  for (const [k, v] of Object.entries(en)) {
    if (!(k in vi)) continue;
    out[k] = isObj(v) && isObj(vi[k]) ? reorder(v, vi[k]) : vi[k];
  }
  for (const k of Object.keys(vi)) if (!(k in out)) out[k] = vi[k];
  return out;
}

function pruneExtra(en, vi) {
  const out = {};
  for (const [k, v] of Object.entries(vi)) {
    if (!(k in en)) continue;
    out[k] = isObj(v) && isObj(en[k]) ? pruneExtra(en[k], v) : v;
  }
  return out;
}

const dirs = (root) => ({ en: join(root, "packages/views/locales/en"), vi: join(root, "packages/views/locales/vi") });
const read = (f) => JSON.parse(readFileSync(f, "utf8"));
const write = (f, o) => writeFileSync(f, JSON.stringify(o, null, 2) + "\n");

export function list(root) {
  const d = dirs(root);
  const out = [];
  for (const f of readdirSync(d.en).filter((n) => n.endsWith(".json")).sort()) {
    let vi = {};
    try {
      vi = read(join(d.vi, f));
    } catch {
      /* whole namespace missing */
    }
    for (const m of missing(read(join(d.en, f)), vi)) out.push({ file: f, path: m.path, en: m.en });
  }
  return out;
}

export function listExtra(root) {
  const d = dirs(root);
  const out = [];
  for (const f of readdirSync(d.vi).filter((n) => n.endsWith(".json")).sort()) {
    let en = {};
    try {
      en = read(join(d.en, f));
    } catch {
      /* namespace gone from English */
    }
    for (const p of extra(en, read(join(d.vi, f)))) out.push({ file: f, path: p });
  }
  return out;
}

export function apply(root, entries, { prune = false } = {}) {
  const d = dirs(root);
  const byFile = new Map();
  for (const e of entries) {
    if (typeof e.vi !== "string" || e.vi === "") throw new Error(`entry ${e.file}:${e.path.join(".")} has no "vi" text`);
    if (!byFile.has(e.file)) byFile.set(e.file, []);
    byFile.get(e.file).push(e);
  }
  for (const [f, es] of byFile) {
    let vi = {};
    try {
      vi = read(join(d.vi, f));
    } catch {
      /* new namespace */
    }
    for (const e of es) setPath(vi, e.path, e.vi);
    const en = read(join(d.en, f));
    write(join(d.vi, f), reorder(en, vi));
  }
  if (prune) {
    for (const f of readdirSync(d.vi).filter((n) => n.endsWith(".json"))) {
      let en;
      try {
        en = read(join(d.en, f));
      } catch {
        continue;
      }
      write(join(d.vi, f), pruneExtra(en, read(join(d.vi, f))));
    }
  }
  return entries.length;
}

export function main(argv, root = process.cwd()) {
  const { values: v } = parseArgs({
    args: argv,
    options: { list: { type: "boolean" }, extra: { type: "boolean" }, apply: { type: "string" }, out: { type: "string" }, prune: { type: "boolean" } },
  });
  if (v.apply) {
    const n = apply(root, JSON.parse(readFileSync(v.apply, "utf8")), { prune: v.prune });
    console.log(`filled ${n} keys in packages/views/locales/vi`);
    return 0;
  }
  if (v.extra) {
    const x = listExtra(root);
    for (const e of x) console.log(`${e.file}: ${e.path.join(".")}`);
    console.log(`${x.length} keys in vi that English does not have`);
    return 0;
  }
  const items = list(root);
  if (v.out) writeFileSync(v.out, JSON.stringify(items, null, 2) + "\n");
  else for (const e of items) console.log(`${e.file}: ${e.path.join(".")} = ${JSON.stringify(e.en)}`);
  console.log(`${items.length} English keys missing from vi`);
  return items.length ? 1 : 0;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main(process.argv.slice(2)));
