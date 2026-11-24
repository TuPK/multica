import assert from "node:assert/strict";
import { test } from "node:test";
import { finish, start } from "./upstream.mjs";
import { makeRepo } from "./testlib.mjs";

const ATTRS = "*.md merge=brand\n";
const POLICY = { excluded: ["server/ee"], regenerate: [{ name: "x", command: "gen-x", order: 1 }], hotFiles: { "hot.go": "S2" } };

function build() {
  const r = makeRepo(ATTRS);
  r.write("both.md", "# Multica\nsame\n");
  r.write("upstream-only.md", "a\n");
  r.write("hot.go", "package x\n// one\n");
  r.write("server/ee/license.go", "package ee\n");
  r.commit("base");
  r.git("branch", "-M", "main");
  r.git("branch", "upstream");
  // fork: brands one file, edits hot.go, deletes ee
  r.write("both.md", "# Delegora\nsame\n");
  r.write("hot.go", "package x\n// fork\n");
  r.git("rm", "-q", "-r", "server/ee");
  r.commit("fork");
  r.git("checkout", "-q", "upstream");
  r.write("both.md", "# Multica Pro\nsame\n");
  r.write("upstream-only.md", "a\nWelcome to Multica\n");
  r.write("hot.go", "package x\n// upstream\n");
  r.write("server/ee/license.go", "package ee\n// changed\n");
  r.commit("upstream");
  r.git("checkout", "-q", "main");
  r.git("update-ref", "refs/remotes/upstream/main", r.git("rev-parse", "upstream").trim());
  return r;
}

test("start merges, removes excluded paths, rebrands upstream-only files and lists real conflicts", () => {
  const r = build();
  const res = start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY, date: "2026-01-02" });
  assert.equal(r.git("rev-parse", "--abbrev-ref", "HEAD").trim(), "chore/sync-upstream-2026-01-02");
  assert.deepEqual(res.conflicts, [{ file: "hot.go", seam: "S2" }]);
  assert.equal(r.read("both.md"), "# Delegora Pro\nsame\n");
  assert.equal(r.read("upstream-only.md"), "a\nWelcome to Delegora\n");
  assert.equal(r.git("ls-files", "server/ee").trim(), "");
  assert.ok(res.log.some((l) => l.includes("excluded by policy")));
  assert.equal(r.git("log", "-1", "--format=%s").trim(), "fork");
  r.cleanup();
});

test("start refuses a dirty tree, a branch other than main, and nothing to merge", () => {
  const r = build();
  r.write("dirty.txt", "x");
  assert.throws(() => start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY }), /not clean/);
  r.git("add", "-A");
  r.git("commit", "-q", "-m", "wip");
  r.git("checkout", "-q", "-b", "other");
  assert.throws(() => start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY }), /start from main/);
  r.git("checkout", "-q", "main");
  r.git("merge", "-q", "--no-edit", "-s", "ours", "upstream");
  assert.throws(() => start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY }), /nothing to merge/);
  r.cleanup();
});

test("finish waits for conflicts, then runs regeneration and the checks in order", () => {
  const r = build();
  start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY, date: "d" });
  assert.throws(() => finish({ root: r.dir, policy: POLICY, run: () => true }), /still unmerged/);
  r.write("hot.go", "package x\n// resolved\n");
  r.git("add", "-A");
  const ran = [];
  const res = finish({ root: r.dir, policy: POLICY, run: (_root, cmd) => (ran.push(cmd), !cmd.includes("brand-check")) });
  assert.equal(ran[0], "gen-x");
  assert.match(ran[1], /brand-check/);
  assert.equal(res.ok, false);
  assert.deepEqual(res.steps.filter((s) => !s.ok).map((s) => s.name), ["brand:check"]);
  r.cleanup();
});

test("finish stops when a file still has conflict markers", () => {
  const r = build();
  start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY, date: "d" });
  r.git("add", "-A"); // adds hot.go with its markers, which clears the unmerged state
  const res = finish({ root: r.dir, policy: POLICY, run: () => true });
  assert.equal(res.ok, false);
  assert.deepEqual(res.steps.map((s) => s.name), ["no conflict markers left in changed files"]);
  assert.match(res.steps[0].note, /hot\.go/);
  r.cleanup();
});

test("finish without a started sync says so", () => {
  const r = build();
  assert.throws(() => finish({ root: r.dir, policy: POLICY, run: () => true }), /no sync in progress/);
  r.cleanup();
});

test("start warns when fork and upstream both added the same path", () => {
  const r = build();
  r.write("dup.go", "package fork\n");
  r.commit("fork adds dup");
  r.git("checkout", "-q", "upstream");
  r.write("dup.go", "package upstream\n");
  r.commit("upstream adds dup");
  r.git("checkout", "-q", "main");
  r.git("update-ref", "refs/remotes/upstream/main", r.git("rev-parse", "upstream").trim());
  const res = start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY, date: "2026-01-03" });
  assert.ok(res.log.some((l) => l.includes("same path added by fork and upstream") && l.includes("dup.go")));
  r.cleanup();
});

test("finish runs the policy's extra checks after the contract", () => {
  const r = build();
  start({ root: r.dir, fetch: false, skipDoctor: true, policy: POLICY, date: "2026-01-04" });
  r.write("hot.go", "package x\n// merged\n");
  r.git("add", "-A");
  const ran = [];
  const res = finish({ root: r.dir, policy: { ...POLICY, finishChecks: [{ name: "golden", command: "run-golden" }] }, run: (_root, cmd) => (ran.push(cmd), true) });
  assert.equal(ran.at(-1), "run-golden");
  assert.equal(res.steps.at(-1).name, "golden");
  r.cleanup();
});
