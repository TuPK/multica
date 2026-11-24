import assert from "node:assert/strict";
import { test } from "node:test";
import { addedLinesByFile, rebrandText, rebrandUpstreamOnly } from "./rebrand.mjs";
import { makeRepo } from "./testlib.mjs";

test("rebrandText brands only lines upstream added and is idempotent", () => {
  const added = new Set(["new Multica line", "multica login"]);
  const once = rebrandText("old Multica line\nnew Multica line\nmultica login\n", added);
  assert.equal(once.text, "old Multica line\nnew Delegora line\nmultica login\n");
  assert.equal(once.changed, 1);
  assert.equal(rebrandText(once.text, added).changed, 0);
});

test("addedLinesByFile reads a -U0 diff", () => {
  const m = addedLinesByFile("diff --git a/x.md b/x.md\n--- a/x.md\n+++ b/x.md\n@@ -1 +1,2 @@\n+one\n+two\n");
  assert.deepEqual([...m.get("x.md")], ["one", "two"]);
});

test("files the fork also changed are left to the merge driver", () => {
  const r = makeRepo("");
  r.write("only-upstream.md", "a\n");
  r.write("both.md", "a\n");
  r.write("locked.md", "a\n");
  r.commit("base");
  r.git("branch", "upstream");
  r.write("both.md", "a fork\n");
  r.commit("fork");
  const fork = r.git("rev-parse", "HEAD").trim();
  r.git("checkout", "-q", "upstream");
  r.write("only-upstream.md", "a\nWelcome to Multica\n");
  r.write("both.md", "a\nWelcome to Multica\n");
  r.commit("upstream");
  const upstream = r.git("rev-parse", "HEAD").trim();
  r.git("checkout", "-q", "main");
  r.merge("upstream");
  r.git("checkout", "--theirs", "--", "both.md");
  const base = r.git("merge-base", fork, upstream).trim();
  const done = rebrandUpstreamOnly({ root: r.dir, base, fork, upstream });
  assert.deepEqual(done, [{ file: "only-upstream.md", lines: 1 }]);
  assert.equal(r.read("only-upstream.md"), "a\nWelcome to Delegora\n");
  assert.equal(r.read("both.md"), "a\nWelcome to Multica\n");
  r.cleanup();
});
