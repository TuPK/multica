#!/usr/bin/env node
// Deterministic grader for the offline S9 tasks (branch A of harness-optimization-plan, Q3). A task is a folder with
// task.json (id, title, request, test command), seed/ (the repository the agent starts from, with its test) and
// solution/ (the files that replace seed/ files to make the test pass).
//
//   node scripts/eval/grade-task.mjs --task <dir>                 check the task itself
//   node scripts/eval/grade-task.mjs --all                        check every task under scripts/eval/tasks
//   node scripts/eval/grade-task.mjs --task <dir> --candidate <d> run the task's test in a candidate work tree
//
// --task checks the grader: the snapshot the agent would get (make-snapshot.mjs, no answer commit in it) must FAIL the
// test, and the same snapshot with solution/ applied must PASS. A task with valid=false is not used for an eval run.
// --candidate runs the test on a directory the agent changed and prints pass or fail.
//
// Output: JSON on stdout. Exit code: 0 all valid (or candidate passed), 1 not valid (or candidate failed), 2 usage error.

import { execFileSync, spawnSync } from "node:child_process";
import { cpSync, existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { makeSnapshot } from "./make-snapshot.mjs";

const TASKS_DIR = fileURLToPath(new URL("./tasks/", import.meta.url));
const GIT = ["-c", "user.name=grader", "-c", "user.email=grader@localhost", "-c", "commit.gpgsign=false"];

export function loadTask(dir) {
  const root = resolve(dir);
  const task = JSON.parse(readFileSync(join(root, "task.json"), "utf8"));
  if (typeof task.id !== "string" || typeof task.test !== "string") throw new Error(`${root}: task.json needs id and test`);
  for (const part of ["seed", "solution"]) {
    if (!existsSync(join(root, part))) throw new Error(`${root}: missing ${part}/`);
  }
  return { ...task, dir: root };
}

/** Runs the task's test command in `cwd`. Pass means exit code 0. */
export function runTest(cwd, command) {
  const res = spawnSync("sh", ["-c", command], { cwd, encoding: "utf8", timeout: 120000 });
  const output = `${res.stdout ?? ""}${res.stderr ?? ""}`.trim().split("\n");
  return { exit: res.status, pass: res.status === 0, tail: output.slice(-6) };
}

const git = (dir, args) => execFileSync("git", ["-C", dir, ...GIT, ...args], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();

/** Copies every file of `from` over `to`, keeping the rest of `to`. */
function overlay(from, to) {
  for (const entry of readdirSync(from)) cpSync(join(from, entry), join(to, entry), { recursive: true });
}

/** Grades one task: a repository with the base commit (seed) and the answer commit (seed plus solution), then snapshot and test. */
export function gradeTask(dir) {
  const task = loadTask(dir);
  const work = mkdtempSync(join(tmpdir(), `grade-${task.id}-`));
  try {
    const repo = join(work, "repo");
    cpSync(join(task.dir, "seed"), repo, { recursive: true });
    git(repo, ["init", "-q"]);
    git(repo, ["add", "-A"]);
    git(repo, ["commit", "-q", "-m", "seed"]);
    const base = git(repo, ["rev-parse", "HEAD"]);
    overlay(join(task.dir, "solution"), repo);
    git(repo, ["add", "-A"]);
    git(repo, ["commit", "-q", "-m", "answer"]);
    const answer = git(repo, ["rev-parse", "HEAD"]);

    const snapshotDir = join(work, "snapshot");
    // The agent starts from the seed commit, the parent of the answer. The answer must not be in the snapshot.
    makeSnapshot({ repo, base, out: snapshotDir, exclude: [answer] });
    const onBase = runTest(snapshotDir, task.test);

    const fixedDir = join(work, "fixed");
    cpSync(snapshotDir, fixedDir, { recursive: true });
    overlay(join(task.dir, "solution"), fixedDir);
    const onFix = runTest(fixedDir, task.test);

    return { id: task.id, title: task.title, base: { exit: onBase.exit, pass: onBase.pass }, fix: { exit: onFix.exit, pass: onFix.pass }, valid: !onBase.pass && onFix.pass, baseOutput: onBase.tail, fixOutput: onFix.tail };
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}

export function candidate(taskDir, candidateDir) {
  const task = loadTask(taskDir);
  const res = runTest(resolve(candidateDir), task.test);
  return { id: task.id, pass: res.pass, exit: res.exit, output: res.tail };
}

function taskDirs() {
  return readdirSync(TASKS_DIR)
    .map((name) => join(TASKS_DIR, name))
    .filter((p) => statSync(p).isDirectory() && existsSync(join(p, "task.json")))
    .sort();
}

function parseArgs(argv) {
  const args = { task: null, all: false, candidate: null };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => {
      const v = argv[++i];
      if (!v) throw new Error(`${a} needs a value`);
      return v;
    };
    if (a === "--task") args.task = next();
    else if (a === "--all") args.all = true;
    else if (a === "--candidate") args.candidate = next();
    else throw new Error(`unknown argument: ${a}`);
  }
  if (args.candidate && !args.task) throw new Error("--candidate needs --task");
  if (!args.all && !args.task) throw new Error("give --task <dir> or --all");
  return args;
}

function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
  } catch (err) {
    process.stderr.write(`grade-task: ${err.message}\nusage: grade-task.mjs (--task <dir> [--candidate <dir>] | --all)\n`);
    process.exit(2);
  }
  if (args.candidate) {
    const res = candidate(args.task, args.candidate);
    process.stdout.write(`${JSON.stringify(res, null, 2)}\n`);
    process.exit(res.pass ? 0 : 1);
  }
  const results = (args.all ? taskDirs() : [args.task]).map((d) => gradeTask(d));
  process.stdout.write(`${JSON.stringify(results, null, 2)}\n`);
  process.exit(results.every((r) => r.valid) ? 0 : 1);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
