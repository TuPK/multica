import assert from "node:assert/strict";
import { test } from "node:test";
import { coveredPaths, inlineChecks, uncovered } from "./coverage.mjs";

test("coveredPaths collects every path a contract check names", () => {
  const set = coveredPaths({ features: [{ checks: [{ kind: "exists", path: "a.go" }, { kind: "matches", path: "b.ts", pattern: "x" }] }] });
  assert.deepEqual([...set].sort(), ["a.go", "b.ts"]);
});

test("uncovered keeps big non-brand-only edits no check names, biggest first, and skips locales and tests", () => {
  const lines = (n) => Array.from({ length: n }, (_, i) => `line ${i}`);
  const files = new Map([
    ["server/a.go", { removed: [], added: lines(40) }],
    ["server/b.go", { removed: [], added: lines(20) }],
    ["server/covered.go", { removed: [], added: lines(50) }],
    ["server/small.go", { removed: [], added: lines(3) }],
    ["packages/views/locales/vi/x.json", { removed: [], added: lines(99) }],
    ["server/c_test.go", { removed: [], added: lines(99) }],
    ["server/brand.go", { removed: ["// Multica"], added: ["// Delegora"] }],
  ]);
  const rows = uncovered({ files, covered: new Set(["server/covered.go"]), min: 15 });
  assert.deepEqual(rows.map((r) => r.file), ["server/a.go", "server/b.go"]);
});

test("inlineChecks prefers top-level declarations, falls back to the longest code line, and escapes it", () => {
  assert.deepEqual(
    inlineChecks("s.go", ["func (h *Handler) listThings(x int) {", "type thingRow struct {", "\tx := 1"]),
    [
      { kind: "matches", path: "s.go", pattern: "\\blistThings\\b" },
      { kind: "matches", path: "s.go", pattern: "\\bthingRow\\b" },
    ],
  );
  const [c] = inlineChecks("t.ts", ["  // a comment that is long enough to be picked", "  doThing(a, b).then((r) => r.value + 1);"]);
  assert.equal(c.pattern, "doThing\\(a,\\s+b\\)\\.then\\(\\(r\\)\\s+=>\\s+r\\.value\\s+\\+\\s+1\\);");
  assert.equal(new RegExp(c.pattern).test("doThing(a, b).then((r) => r.value + 1);"), true);
  assert.deepEqual(inlineChecks("u.ts", ["x = 1;"]), []);
});
