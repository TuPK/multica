import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { test } from "node:test";
import { classify } from "./classify.mjs";
import { simulate } from "./simulate.mjs";
import { makeRepo } from "./testlib.mjs";

const ATTRS = "*.md merge=brand\npackages/views/locales/**/*.json merge=jsonmerge\n";
const j = (o) => JSON.stringify(o, null, 2) + "\n";

function build() {
  const r = makeRepo(ATTRS);
  r.write("packages/views/locales/en/a.json", j({ a: "1", z: "9" }));
  r.write("notes.md", "# Multica\nsame\n");
  r.write("code.go", "package x\n// one\n");
  r.commit("base");
  const base = r.git("rev-parse", "HEAD").trim();
  r.git("branch", "upstream");
  r.write("packages/views/locales/en/a.json", j({ a: "1", forkKey: "f", z: "9" }));
  r.write("notes.md", "# Delegora\nsame\n");
  r.write("code.go", "package x\n// fork\n");
  r.commit("fork");
  r.git("checkout", "-q", "upstream");
  r.write("packages/views/locales/en/a.json", j({ a: "1", upKey: "u", z: "9" }));
  r.write("notes.md", "# Multica Pro\nsame\nmore Multica\n");
  r.write("code.go", "package x\n// upstream\n");
  r.commit("upstream");
  r.git("checkout", "-q", "main");
  return { r, base };
}

test("drivers off reports every file both sides touched; drivers on leaves only the real conflict", () => {
  const { r } = build();
  const off = simulate({ root: r.dir, fork: "main", upstream: "upstream", drivers: false });
  assert.deepEqual(off.conflicts.sort(), ["code.go", "notes.md", "packages/views/locales/en/a.json"]);
  const on = simulate({ root: r.dir, fork: "main", upstream: "upstream", drivers: true });
  assert.deepEqual(on.conflicts, ["code.go"]);
  assert.deepEqual(on.byClass, { "go-code": 1 });
  r.cleanup();
});

test("simulate leaves no worktree, no merge in progress and the checkout untouched", () => {
  const { r } = build();
  const before = r.git("status", "--porcelain");
  simulate({ root: r.dir, fork: "main", upstream: "upstream", drivers: true });
  assert.equal(r.git("status", "--porcelain"), before);
  assert.equal(r.git("worktree", "list").trim().split("\n").length, 1);
  assert.ok(!existsSync(`${r.dir}/.git/MERGE_HEAD`));
  r.cleanup();
});

test("--compare flags a clean file that differs from the real merge", () => {
  const { r } = build();
  r.git("checkout", "-q", "-b", "real");
  try {
    r.git("merge", "--no-commit", "--no-ff", "upstream");
  } catch {}
  r.write("code.go", "package x\n// resolved\n");
  r.write("notes.md", "# Delegora\nsame\nmore Delegora\nEXTRA\n");
  r.write("packages/views/locales/en/a.json", j({ a: "1", forkKey: "f", upKey: "u", z: "9" }));
  r.commit("real merge");
  const res = simulate({ root: r.dir, fork: "main", upstream: "upstream", drivers: true, compareWith: "real" });
  assert.deepEqual(res.compared.differ.map((d) => d.file), ["notes.md"]);
  assert.equal(res.compared.checked, 2);
  r.cleanup();
});

test("classify() puts paths in the buckets of the spec", () => {
  assert.equal(classify("packages/views/locales/ko/a.json"), "locales");
  assert.equal(classify("server/pkg/db/generated/models.go"), "generated/lock");
  assert.equal(classify("apps/docs/content/docs/a.mdx"), "docs/md");
  assert.equal(classify("server/x_test.go"), "tests");
  assert.equal(classify("server/x.go"), "go-code");
  assert.equal(classify("packages/a/b.tsx"), "ts-code");
  assert.equal(classify("scripts/x.mjs"), "config/build");
});
