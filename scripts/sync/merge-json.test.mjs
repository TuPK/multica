import assert from "node:assert/strict";
import { test } from "node:test";
import { makeRepo } from "./testlib.mjs";

const ATTRS = "*.json merge=jsonmerge\n";
const j = (o) => JSON.stringify(o, null, 2) + "\n";

function scenario(base, fork, upstream, path = "a.json") {
  const r = makeRepo(ATTRS);
  r.write(path, typeof base === "string" ? base : j(base));
  r.commit("base");
  r.git("branch", "upstream");
  r.write(path, typeof fork === "string" ? fork : j(fork));
  r.commit("fork");
  r.git("checkout", "-q", "upstream");
  r.write(path, typeof upstream === "string" ? upstream : j(upstream));
  r.commit("upstream");
  r.git("checkout", "-q", "main");
  const conflicts = r.merge("upstream");
  const raw = r.read(path);
  r.cleanup();
  return { conflicts, raw, json: conflicts.length ? null : JSON.parse(raw) };
}

test("both sides adding neighbouring keys merge, keys from both sides survive, outer keys keep their place", () => {
  const { conflicts, json, raw } = scenario(
    { a: "1", z: "9" },
    { a: "1", forkKey: "f", z: "9" },
    { a: "1", upstreamKey: "u", z: "9" },
  );
  assert.deepEqual(conflicts, []);
  assert.deepEqual(Object.keys(json).sort(), ["a", "forkKey", "upstreamKey", "z"]);
  assert.equal(Object.keys(json)[0], "a");
  assert.equal(Object.keys(json).at(-1), "z");
  assert.ok(raw.endsWith("\n"));
});

test("fork deleted a key while upstream added the next one", () => {
  const { conflicts, json } = scenario(
    { a: "1", b: "2", c: "3" },
    { a: "1", c: "3" },
    { a: "1", b: "2", new: "n", c: "3" },
  );
  assert.deepEqual(conflicts, []);
  assert.deepEqual(json, { a: "1", new: "n", c: "3" });
});

test("nested objects merge recursively", () => {
  const { conflicts, json } = scenario(
    { s: { x: { p: "1" } } },
    { s: { x: { p: "1", f: "fork" } } },
    { s: { x: { p: "1", u: "up" }, y: "new" } },
  );
  assert.deepEqual(conflicts, []);
  assert.deepEqual(json, { s: { x: { p: "1", f: "fork", u: "up" }, y: "new" } });
});

test("strings from upstream are branded, fork strings are kept as written", () => {
  const { conflicts, json } = scenario(
    { a: "Multica one", b: "Multica two" },
    { a: "Delegora one", b: "Multica two", f: "Delegora fork" },
    { a: "Multica one", b: "Multica two", u: "Welcome to Multica", p: "multica login" },
  );
  assert.deepEqual(conflicts, []);
  assert.equal(json.a, "Delegora one");
  assert.equal(json.u, "Welcome to Delegora");
  assert.equal(json.p, "multica login");
});

test("brand-only fork edit does not conflict with an upstream edit of the same value", () => {
  const { conflicts, json } = scenario({ a: "Multica one" }, { a: "Delegora one" }, { a: "Multica two" });
  assert.deepEqual(conflicts, []);
  assert.equal(json.a, "Delegora two");
});

test("both sides editing the same value is a real conflict", () => {
  const { conflicts, raw } = scenario({ a: "x" }, { a: "y" }, { a: "z" });
  assert.deepEqual(conflicts, ["a.json"]);
  assert.match(raw, /<<<<<<</);
});

test("one side deleting a key the other edited is a real conflict", () => {
  const { conflicts } = scenario({ a: "x", b: "1" }, { b: "1" }, { a: "changed", b: "1" });
  assert.deepEqual(conflicts, ["a.json"]);
});

test("string arrays merge element by element", () => {
  const { conflicts, json } = scenario(
    { pages: ["a", "b", "c", "d"] },
    { pages: ["a", "fork", "b", "c", "d"] },
    { pages: ["a", "b", "c", "d", "upstream"] },
  );
  assert.deepEqual(conflicts, []);
  assert.deepEqual(json.pages, ["a", "fork", "b", "c", "d", "upstream"]);
});

test("both sides inserting at the same spot of a string array keeps both, ours first", () => {
  const { conflicts, json } = scenario(
    { pages: ["a", "b", "c"] },
    { pages: ["a", "b", "f1", "f2", "c"] },
    { pages: ["a", "b", "u1", "c"] },
  );
  assert.deepEqual(conflicts, []);
  assert.deepEqual(json.pages, ["a", "b", "f1", "f2", "u1", "c"]);
});

test("a deletion that overlaps the other side's insertion stays a conflict", () => {
  const { conflicts } = scenario({ pages: ["a", "b", "c"] }, { pages: ["a", "c"] }, { pages: ["a", "b", "x", "c"] });
  assert.deepEqual(conflicts, ["a.json"]);
});

test("a fork file whose layout is not canonical falls back to a line merge", () => {
  const compact = '{\n  "pages": ["a", "b"]\n}\n';
  const { conflicts, raw } = scenario(compact, '{\n  "pages": ["a", "b", "f"]\n}\n', '{\n  "pages": ["a", "b", "u"]\n}\n');
  assert.deepEqual(conflicts, ["a.json"]);
  assert.match(raw, /<<<<<<</);
});

test("invalid JSON never produces output", () => {
  const { conflicts } = scenario('{"a": 1}\n', '{"a": 1, bad\n', '{"a": 2}\n');
  assert.deepEqual(conflicts, ["a.json"]);
});
