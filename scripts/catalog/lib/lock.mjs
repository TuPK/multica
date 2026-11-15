// A cross-process lock for the build. `build` removes and rewrites the generated directories, so two
// builds that overlap (turbo dev, `make test`, `make multica` and an agent's go test all run it) fail with
// ENOTEMPTY or ENOENT inside rmSync/writeFileSync. The lock is a directory: mkdir is atomic on every
// platform, and the owner's pid lets a waiter take over from a build that was killed.
import { mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { createHash } from "node:crypto";
import { join } from "node:path";

const POLL_MS = 50;
// A build takes about a second; a lock older than this belongs to a build that never released it.
const STALE_MS = 120_000;

export function defaultLockDir(repoRoot) {
  return join(tmpdir(), `multica-catalog-build-${createHash("sha1").update(repoRoot).digest("hex").slice(0, 12)}.lock`);
}

function sleep(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

function ownerAlive(lockDir) {
  try {
    const pid = Number(readFileSync(join(lockDir, "pid"), "utf8"));
    if (!Number.isInteger(pid) || pid <= 0) return false;
    process.kill(pid, 0);
    return true;
  } catch (err) {
    // EPERM: the process exists and belongs to someone else.
    return err.code === "EPERM";
  }
}

function lockIsStale(lockDir) {
  try {
    if (Date.now() - statSync(lockDir).mtimeMs > STALE_MS) return true;
  } catch {
    return false;
  }
  // The pid file is written right after mkdir; a lock without one is still being taken, so give it a moment.
  try {
    statSync(join(lockDir, "pid"));
  } catch {
    return false;
  }
  return !ownerAlive(lockDir);
}

// Runs fn while holding the lock. Throws when the lock cannot be taken within timeoutMs.
export function withBuildLock(lockDir, fn, { timeoutMs = STALE_MS } = {}) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      mkdirSync(lockDir);
      break;
    } catch (err) {
      if (err.code !== "EEXIST") throw err;
      if (lockIsStale(lockDir)) {
        rmSync(lockDir, { recursive: true, force: true });
        continue;
      }
      if (Date.now() >= deadline) throw new Error(`catalog build lock ${lockDir} is held by another build`);
      sleep(POLL_MS);
    }
  }
  try {
    writeFileSync(join(lockDir, "pid"), String(process.pid));
    return fn();
  } finally {
    rmSync(lockDir, { recursive: true, force: true });
  }
}
