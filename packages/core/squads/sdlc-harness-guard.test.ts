// @vitest-environment node
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const scriptsDir = fileURLToPath(new URL("../../../catalog/skills/builtin/sdlc-harness/scripts/", import.meta.url));
const installer = join(scriptsDir, "install-guard.sh");

const dirs: string[] = [];
afterEach(() => {
  for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

const GIT = ["-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"];
function git(dir: string, ...args: string[]) {
  return execFileSync("git", ["-C", dir, ...GIT, ...args], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

/** A bare remote and a clone with one commit on main, the way a squad workspace is set up. */
function remoteAndClone() {
  const root = mkdtempSync(join(tmpdir(), "guard-"));
  dirs.push(root);
  const remote = join(root, "remote.git");
  const clone = join(root, "clone");
  execFileSync("git", ["init", "-q", "--bare", remote]);
  mkdirSync(clone);
  git(clone, "init", "-q");
  git(clone, "symbolic-ref", "HEAD", "refs/heads/main");
  writeFileSync(join(clone, "a.txt"), "a\n");
  git(clone, "add", "-A");
  git(clone, "commit", "-q", "-m", "init");
  git(clone, "remote", "add", "origin", remote);
  git(clone, "push", "-q", "origin", "HEAD:main");
  return { remote, clone };
}

function install(clone: string, ...flags: string[]) {
  return spawnSync("sh", [installer, "--repo", clone, ...flags], { encoding: "utf8" });
}

function pushMain(clone: string, role?: string) {
  const env: NodeJS.ProcessEnv = { ...process.env };
  if (role === undefined) delete env.SDLC_ROLE;
  else env.SDLC_ROLE = role;
  return spawnSync("git", ["-C", clone, ...GIT, "push", "origin", "HEAD:main"], { encoding: "utf8", env });
}

describe("push guard (W5.a)", () => {
  it("refuses a push to main when SDLC_ROLE is not qa, and leaves main unchanged", () => {
    const { remote, clone } = remoteAndClone();
    expect(install(clone).status).toBe(0);
    writeFileSync(join(clone, "b.txt"), "b\n");
    git(clone, "add", "-A");
    git(clone, "commit", "-q", "-m", "change");
    const before = execFileSync("git", ["-C", remote, "rev-parse", "main"], { encoding: "utf8" }).trim();
    const res = pushMain(clone);
    expect(res.status).not.toBe(0);
    expect(res.stderr).toContain("refused");
    const after = execFileSync("git", ["-C", remote, "rev-parse", "main"], { encoding: "utf8" }).trim();
    expect(after).toBe(before);
  });

  it("accepts a push to main when the QA landing step sets SDLC_ROLE=qa", () => {
    const { remote, clone } = remoteAndClone();
    install(clone);
    writeFileSync(join(clone, "b.txt"), "b\n");
    git(clone, "add", "-A");
    git(clone, "commit", "-q", "-m", "landing: b");
    const res = pushMain(clone, "qa");
    expect(res.status, res.stderr).toBe(0);
    expect(execFileSync("git", ["-C", remote, "rev-parse", "main"], { encoding: "utf8" }).trim()).toBe(git(clone, "rev-parse", "HEAD"));
  });

  it("does not block a push to a branch", () => {
    const { clone } = remoteAndClone();
    install(clone);
    git(clone, "checkout", "-q", "-b", "agent/dev/x");
    writeFileSync(join(clone, "c.txt"), "c\n");
    git(clone, "add", "-A");
    git(clone, "commit", "-q", "-m", "work");
    const res = spawnSync("git", ["-C", clone, ...GIT, "push", "origin", "HEAD:agent/dev/x"], { encoding: "utf8" });
    expect(res.status, res.stderr).toBe(0);
  });

  it("refuses deleting main without SDLC_ROLE=qa", () => {
    const { clone } = remoteAndClone();
    install(clone);
    const res = spawnSync("git", ["-C", clone, ...GIT, "push", "origin", ":main"], { encoding: "utf8", env: { ...process.env, SDLC_ROLE: "" } });
    expect(res.status).not.toBe(0);
    expect(res.stderr).toContain("refused");
  });

  it("is removed by one command, after which a push to main is allowed again", () => {
    const { remote, clone } = remoteAndClone();
    install(clone);
    expect(existsSync(join(clone, ".git/hooks/pre-push"))).toBe(true);
    const removed = install(clone, "--remove");
    expect(removed.status).toBe(0);
    expect(existsSync(join(clone, ".git/hooks/pre-push"))).toBe(false);
    writeFileSync(join(clone, "d.txt"), "d\n");
    git(clone, "add", "-A");
    git(clone, "commit", "-q", "-m", "after rollback");
    expect(pushMain(clone).status).toBe(0);
    expect(execFileSync("git", ["-C", remote, "rev-parse", "main"], { encoding: "utf8" }).trim()).toBe(git(clone, "rev-parse", "HEAD"));
  });

  it("refuses to overwrite a pre-push hook it did not write", () => {
    const { clone } = remoteAndClone();
    mkdirSync(join(clone, ".git/hooks"), { recursive: true });
    writeFileSync(join(clone, ".git/hooks/pre-push"), "#!/bin/sh\necho mine\n");
    const res = install(clone);
    expect(res.status).toBe(1);
    expect(res.stderr).toContain("not the SDLC guard");
    expect(readFileSync(join(clone, ".git/hooks/pre-push"), "utf8")).toContain("echo mine");
  });
});
