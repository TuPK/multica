import assert from "node:assert/strict";
import { test } from "node:test";
import { brandOnly, measure, parseDiff } from "./surface.mjs";
import { makeRepo } from "./testlib.mjs";

test("parseDiff splits removed and added lines per file", () => {
  const files = parseDiff("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old Multica\n+new Delegora\n");
  assert.deepEqual(files.get("x"), { removed: ["old Multica"], added: ["new Delegora"] });
});

test("brandOnly ignores spelling and order but not content", () => {
  assert.ok(brandOnly({ removed: ["a Multica", "b"], added: ["b", "a Delegora"] }));
  assert.ok(!brandOnly({ removed: ["a Multica"], added: ["a Delegora!"] }));
  assert.ok(!brandOnly({ removed: ["a"], added: [] }));
});

test("measure counts added, modified, deleted and brand-only files", () => {
  const r = makeRepo("");
  r.write("same.md", "x\n");
  r.write("brand.md", "Multica\n");
  r.write("real.md", "one\n");
  r.write("gone.md", "bye\n");
  r.commit("upstream");
  r.git("branch", "upstream");
  r.write("brand.md", "Delegora\n");
  r.write("real.md", "two\n");
  r.write("new.md", "fork only\n");
  r.git("rm", "-q", "gone.md");
  r.commit("fork");
  const m = measure({ upstream: "upstream", head: "HEAD", git: (...a) => r.git(...a) });
  assert.deepEqual({ a: m.added, m: m.modified, d: m.deleted, b: m.brandOnly, s: m.substantive }, { a: 1, m: 2, d: 1, b: 1, s: 1 });
  r.cleanup();
});
