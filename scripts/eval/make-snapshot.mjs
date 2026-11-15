#!/usr/bin/env node
// Makes an offline evaluation snapshot of one commit of a repository. The snapshot is a new repository with a single
// commit whose tree is the base commit's tree. The source history is not copied, so the answer commit cannot be read
// from the snapshot's git data.
//
//   node scripts/eval/make-snapshot.mjs --repo <dir> --base <commit> --out <dir> [--exclude <answer sha>]...
//
//   --repo      source repository
//   --base      commit the agent starts from: the parent of the commit that holds the answer
//   --out       directory to create (it must not exist, or be empty)
//   --exclude   sha of a commit the snapshot must not contain (repeatable). Refuses if the base descends from it.
//
// The manifest `<out>.snapshot.json` (source name, base sha, excluded shas, time) is written next to the snapshot,
// never inside it, so the agent cannot read the excluded shas.
// Exit code: 0 ok, 1 the snapshot failed a check (it is removed), 2 usage error.

import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const GIT_IDENTITY = ["-c", "user.name=snapshot", "-c", "user.email=snapshot@localhost", "-c", "commit.gpgsign=false"];

function git(dir, args) {
  return execFileSync("git", ["-C", dir, ...args], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
}

function fullSha(repo, ref) {
  return git(repo, ["rev-parse", "--verify", "--quiet", `${ref}^{commit}`]).trim();
}

/** Throws unless the snapshot is one commit and none of the excluded commits exists in it. */
function verify(dest, excluded) {
  const commits = git(dest, ["rev-list", "--all"]).trim().split("\n").filter(Boolean);
  if (commits.length !== 1) throw new Error(`snapshot has ${commits.length} commits, expected 1`);
  for (const sha of excluded) {
    const probe = spawnSync("git", ["-C", dest, "cat-file", "-e", `${sha}^{commit}`], { stdio: "ignore" });
    if (probe.status === 0) throw new Error(`excluded commit ${sha.slice(0, 12)} exists in the snapshot`);
  }
}

export function makeSnapshot({ repo, base, out, exclude = [] }) {
  const src = resolve(repo);
  const dest = resolve(out);
  if (existsSync(dest) && readdirSync(dest).length > 0) throw new Error(`${dest} exists and is not empty`);
  const baseSha = fullSha(src, base);
  const excluded = exclude.map((ref) => fullSha(src, ref));
  for (const sha of excluded) {
    if (sha === baseSha) throw new Error("the base commit is an excluded commit");
    const descends = spawnSync("git", ["-C", src, "merge-base", "--is-ancestor", sha, baseSha], { stdio: "ignore" }).status === 0;
    if (descends) throw new Error(`the base commit descends from excluded commit ${sha.slice(0, 12)}: pick the parent of the answer`);
  }

  const work = mkdtempSync(join(tmpdir(), "snapshot-"));
  try {
    const tar = join(work, "base.tar");
    git(src, ["archive", "--format=tar", `--output=${tar}`, baseSha]);
    mkdirSync(dest, { recursive: true });
    execFileSync("tar", ["-xf", tar, "-C", dest]);
    git(dest, ["init", "-q"]);
    git(dest, ["add", "-A"]);
    git(dest, [...GIT_IDENTITY, "commit", "-q", "--allow-empty", "-m", "snapshot"]);
    verify(dest, excluded);
  } catch (err) {
    rmSync(dest, { recursive: true, force: true });
    throw err;
  } finally {
    rmSync(work, { recursive: true, force: true });
  }

  const manifest = {
    schema: 1,
    source: basename(src),
    base: baseSha,
    excluded,
    createdAt: new Date().toISOString(),
  };
  writeFileSync(`${dest}.snapshot.json`, `${JSON.stringify(manifest, null, 2)}\n`);
  return { out: dest, manifest: `${dest}.snapshot.json`, ...manifest };
}

function parseArgs(argv) {
  const args = { repo: null, base: null, out: null, exclude: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => {
      const v = argv[++i];
      if (!v) throw new Error(`${a} needs a value`);
      return v;
    };
    if (a === "--repo") args.repo = next();
    else if (a === "--base") args.base = next();
    else if (a === "--out") args.out = next();
    else if (a === "--exclude") args.exclude.push(next());
    else throw new Error(`unknown argument: ${a}`);
  }
  if (!args.repo || !args.base || !args.out) throw new Error("--repo, --base and --out are required");
  return args;
}

function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
  } catch (err) {
    process.stderr.write(`make-snapshot: ${err.message}\nusage: make-snapshot.mjs --repo <dir> --base <commit> --out <dir> [--exclude <sha>]...\n`);
    process.exit(2);
  }
  try {
    const result = makeSnapshot({ repo: args.repo, base: args.base, out: args.out, exclude: args.exclude });
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  } catch (err) {
    process.stderr.write(`make-snapshot: ${err.message}\n`);
    process.exit(1);
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
