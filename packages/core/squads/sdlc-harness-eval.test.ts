// @vitest-environment node
import { execFileSync, spawnSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const scriptsDir = fileURLToPath(new URL("../../../scripts/eval/", import.meta.url));
const snapshotScript = join(scriptsDir, "make-snapshot.mjs");
const graderScript = join(scriptsDir, "grade-task.mjs");
const taskDir = (id: string) => join(scriptsDir, "tasks", id);

const dirs: string[] = [];
afterEach(() => {
  for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

function tempDir(prefix: string) {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  dirs.push(dir);
  return dir;
}

function git(dir: string, ...args: string[]) {
  return execFileSync("git", ["-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false", ...args], {
    encoding: "utf8",
  }).trim();
}

function put(dir: string, files: Record<string, string>) {
  for (const [rel, text] of Object.entries(files)) {
    mkdirSync(dirname(join(dir, rel)), { recursive: true });
    writeFileSync(join(dir, rel), text);
  }
}

/** A repository with a base commit and a later answer commit, the way an eval task's history looks. */
function historyRepo() {
  const repo = tempDir("snap-src-");
  git(repo, "init", "-q");
  put(repo, { "src/sum.mjs": "export const sum = () => 0;\n", "README.md": "# demo\n" });
  git(repo, "add", "-A");
  git(repo, "commit", "-q", "-m", "base");
  const base = git(repo, "rev-parse", "HEAD");
  put(repo, { "src/sum.mjs": "export const sum = (xs) => xs.reduce((a, b) => a + b, 0);\n", "answer.txt": "the answer\n" });
  git(repo, "add", "-A");
  git(repo, "commit", "-q", "-m", "answer");
  const answer = git(repo, "rev-parse", "HEAD");
  return { repo, base, answer };
}

function snapshot(args: string[]) {
  return spawnSync("node", [snapshotScript, ...args], { encoding: "utf8" });
}

describe("make-snapshot", () => {
  it("builds a snapshot with one commit, the base tree, and no answer file", () => {
    const { repo, base, answer } = historyRepo();
    const out = join(tempDir("snap-out-"), "task");
    const res = snapshot(["--repo", repo, "--base", base, "--out", out, "--exclude", answer]);
    expect(res.status, res.stderr).toBe(0);
    expect(git(out, "rev-list", "--all").split("\n")).toHaveLength(1);
    expect(existsSync(join(out, "answer.txt"))).toBe(false);
    expect(readFileSync(join(out, "src/sum.mjs"), "utf8")).toContain("() => 0");
  });

  it("keeps the answer sha out of the snapshot, and writes the manifest next to it", () => {
    const { repo, base, answer } = historyRepo();
    const out = join(tempDir("snap-out-"), "task");
    expect(snapshot(["--repo", repo, "--base", base, "--out", out, "--exclude", answer]).status).toBe(0);
    const probe = spawnSync("git", ["-C", out, "cat-file", "-e", `${answer}^{commit}`], { stdio: "ignore" });
    expect(probe.status).not.toBe(0);
    const manifest = JSON.parse(readFileSync(`${out}.snapshot.json`, "utf8"));
    expect(manifest).toMatchObject({ schema: 1, base, excluded: [answer] });
    expect(existsSync(join(out, "snapshot.json"))).toBe(false);
  });

  it("refuses a base that descends from the answer, so the answer cannot be in its history", () => {
    const { repo, answer } = historyRepo();
    put(repo, { "later.txt": "later\n" });
    git(repo, "add", "-A");
    git(repo, "commit", "-q", "-m", "later");
    const later = git(repo, "rev-parse", "HEAD");
    const out = join(tempDir("snap-out-"), "task");
    const res = snapshot(["--repo", repo, "--base", later, "--out", out, "--exclude", answer]);
    expect(res.status).toBe(1);
    expect(res.stderr).toContain("descends from excluded commit");
    expect(existsSync(out)).toBe(false);
  });

  it("refuses an output directory that already holds files", () => {
    const { repo, base, answer } = historyRepo();
    const out = tempDir("snap-out-");
    put(out, { "old.txt": "x" });
    const res = snapshot(["--repo", repo, "--base", base, "--out", out, "--exclude", answer]);
    expect(res.status).toBe(1);
    expect(res.stderr).toContain("not empty");
  });

  it("exits 2 on missing arguments", () => {
    expect(snapshot(["--repo", "."]).status).toBe(2);
  });
});

describe("grade-task (branch A: deterministic tasks)", () => {
  it("grades every task: the snapshot fails its test, and the solution makes it pass", () => {
    const res = spawnSync("node", [graderScript, "--all"], { encoding: "utf8" });
    expect(res.status, res.stderr).toBe(0);
    const results = JSON.parse(res.stdout) as { id: string; base: { pass: boolean }; fix: { pass: boolean }; valid: boolean }[];
    expect(results.map((r) => r.id)).toEqual(["parse-duration", "slugify", "sum-open"]);
    for (const r of results) {
      expect(r.base.pass, `${r.id} on base`).toBe(false);
      expect(r.fix.pass, `${r.id} on fix`).toBe(true);
      expect(r.valid, r.id).toBe(true);
    }
  });

  it("marks a task invalid when its solution does not make the test pass", () => {
    const task = tempDir("grade-bad-");
    put(task, {
      "task.json": JSON.stringify({ id: "broken", title: "broken", request: "x", test: "node --test test/t.test.mjs" }),
      "seed/src/x.mjs": "export const x = 1;\n",
      "seed/test/t.test.mjs": 'import { test } from "node:test";\nimport assert from "node:assert/strict";\nimport { x } from "../src/x.mjs";\ntest("x is 2", () => assert.equal(x, 2));\n',
      "solution/src/x.mjs": "export const x = 3;\n",
    });
    const res = spawnSync("node", [graderScript, "--task", task], { encoding: "utf8" });
    expect(res.status).toBe(1);
    const [result] = JSON.parse(res.stdout) as { valid: boolean; fix: { pass: boolean } }[];
    expect(result).toMatchObject({ valid: false, fix: { pass: false } });
  });

  it("runs the test on a candidate work tree: the solution passes, the seed fails", () => {
    const work = tempDir("grade-cand-");
    cpSync(join(taskDir("slugify"), "seed"), work, { recursive: true });
    const seed = spawnSync("node", [graderScript, "--task", taskDir("slugify"), "--candidate", work], { encoding: "utf8" });
    expect(seed.status).toBe(1);
    expect(JSON.parse(seed.stdout)).toMatchObject({ id: "slugify", pass: false });
    cpSync(join(taskDir("slugify"), "solution"), work, { recursive: true });
    const fixed = spawnSync("node", [graderScript, "--task", taskDir("slugify"), "--candidate", work], { encoding: "utf8" });
    expect(fixed.status).toBe(0);
    expect(JSON.parse(fixed.stdout)).toMatchObject({ id: "slugify", pass: true });
  });

  it("exits 2 on bad arguments", () => {
    expect(spawnSync("node", [graderScript], { encoding: "utf8" }).status).toBe(2);
    expect(spawnSync("node", [graderScript, "--candidate", "."], { encoding: "utf8" }).status).toBe(2);
  });
});
