// Test helper: builds throwaway git repos wired to the sync merge drivers.
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const SYNC_DIR = dirname(fileURLToPath(import.meta.url));

export function makeRepo(attributes) {
  const dir = mkdtempSync(join(tmpdir(), "sync-test-"));
  const git = (...args) =>
    execFileSync("git", args, { cwd: dir, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
  git("init", "-q", "-b", "main");
  git("config", "user.email", "t@example.com");
  git("config", "user.name", "t");
  git("config", "commit.gpgsign", "false");
  git("config", "rerere.enabled", "false");
  git("config", "merge.brand.driver", `node ${SYNC_DIR}/merge-brand.mjs %O %A %B %L %P`);
  git("config", "merge.jsonmerge.driver", `node ${SYNC_DIR}/merge-json.mjs %O %A %B %L`);
  git("config", "merge.ours.driver", "true");
  writeFileSync(join(dir, ".gitattributes"), attributes);
  const write = (path, text) => {
    mkdirSync(dirname(join(dir, path)), { recursive: true });
    writeFileSync(join(dir, path), text);
  };
  const commit = (message) => {
    git("add", "-A");
    git("commit", "-q", "-m", message);
  };
  return {
    dir,
    git,
    write,
    commit,
    read: (path) => readFileSync(join(dir, path), "utf8"),
    /** Merge `branch` into the current branch; returns the unmerged file list. */
    merge(branch) {
      try {
        git("merge", "--no-commit", "--no-ff", branch);
      } catch {
        /* conflicts are expected */
      }
      return git("diff", "--name-only", "--diff-filter=U").split("\n").filter(Boolean);
    },
    cleanup: () => rmSync(dir, { recursive: true, force: true }),
  };
}
