import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { apply, extra, list, listExtra, missing, reorder } from "./vi-worklist.mjs";

function fixture(en, vi) {
  const root = mkdtempSync(join(tmpdir(), "vi-"));
  mkdirSync(join(root, "packages/views/locales/en"), { recursive: true });
  mkdirSync(join(root, "packages/views/locales/vi"), { recursive: true });
  for (const [f, o] of Object.entries(en)) writeFileSync(join(root, "packages/views/locales/en", f), JSON.stringify(o, null, 2) + "\n");
  for (const [f, o] of Object.entries(vi)) writeFileSync(join(root, "packages/views/locales/vi", f), JSON.stringify(o, null, 2) + "\n");
  return root;
}
const viFile = (root, f) => JSON.parse(readFileSync(join(root, "packages/views/locales/vi", f), "utf8"));

test("missing and extra work on nested objects and plural suffixes", () => {
  const en = { a: "1", n: { one: "x", other: "y", deep: { k: "z" } } };
  const vi = { a: "m", n: { one: "x" }, old: "q" };
  assert.deepEqual(missing(en, vi).map((m) => m.path.join(".")), ["n.other", "n.deep.k"]);
  assert.deepEqual(extra(en, vi).map((p) => p.join(".")), ["old"]);
});

test("list covers a whole missing namespace; apply fills keys in English order", () => {
  const root = fixture(
    { "a.json": { first: "1", second: "2", third: "3" }, "b.json": { only: "x" } },
    { "a.json": { first: "m", third: "c" } },
  );
  const items = list(root);
  assert.deepEqual(items.map((i) => `${i.file}:${i.path.join(".")}`), ["a.json:second", "b.json:only"]);
  apply(root, items.map((i) => ({ ...i, vi: `vi-${i.path.join(".")}` })));
  assert.deepEqual(Object.keys(viFile(root, "a.json")), ["first", "second", "third"]);
  assert.equal(viFile(root, "b.json").only, "vi-only");
  assert.deepEqual(list(root), []);
});

test("apply refuses an entry without translation and prune drops keys English removed", () => {
  const root = fixture({ "a.json": { k: "1" } }, { "a.json": { k: "m", gone: "x" } });
  assert.throws(() => apply(root, [{ file: "a.json", path: ["k2"], en: "2" }]), /no "vi" text/);
  assert.deepEqual(listExtra(root).map((e) => e.path.join(".")), ["gone"]);
  apply(root, [], { prune: true });
  assert.deepEqual(viFile(root, "a.json"), { k: "m" });
});

test("English-only plural forms are not required in vi", () => {
  const en = { items_one: "{{count}} item", items_other: "{{count}} items", plain: "x" };
  assert.deepEqual(missing(en, { plain: "x" }).map((m) => m.path[0]), ["items_other"]);
});

test("reorder keeps keys English lacks at the end", () => {
  assert.deepEqual(Object.keys(reorder({ a: 1, b: 2 }, { b: 1, z: 1, a: 1 })), ["a", "b", "z"]);
});
