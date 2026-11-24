// Post-merge brand pass for files only upstream changed. Git calls a merge driver only
// when both sides edited a file; a file upstream alone edited arrives with upstream's
// spelling. This brands exactly the lines upstream added, so nothing else in the file
// is touched. See docs/specs/upstream-sync/spec.md section 3.1b.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { brand } from "./brand-rules.mjs";

const TEXT = /\.(md|mdx|go|ts|tsx|mjs|jsonc?|ya?ml|sh|txt)$/;
const SKIP = /(^|\/)(generated|node_modules)\/|pnpm-lock\.yaml$|(^|\/)(server|packages)\/ee\//;

/** Lines `+` in `git diff -U0 base upstream`, per file. */
export function addedLinesByFile(diffText) {
  const out = new Map();
  let file = null;
  for (const line of diffText.split("\n")) {
    const m = /^diff --git a\/.+ b\/(.+)$/.exec(line);
    if (m) {
      file = m[1];
      out.set(file, new Set());
    } else if (file && line.startsWith("+") && !line.startsWith("+++")) out.get(file).add(line.slice(1));
  }
  return out;
}

export function rebrandText(text, added) {
  let changed = 0;
  const next = text
    .split("\n")
    .map((line) => {
      if (!added.has(line)) return line;
      const b = brand(line);
      if (b !== line) changed += 1;
      return b;
    })
    .join("\n");
  return { text: next, changed };
}

/**
 * Brands upstream-added lines in files the fork left alone since `base`.
 * Returns [{file, lines}] for every file it rewrote.
 */
export function rebrandUpstreamOnly({ root, base, fork, upstream }) {
  const git = (...a) => execFileSync("git", a, { cwd: root, encoding: "utf8", maxBuffer: 1 << 30 });
  const forkTouched = new Set(git("diff", "--name-only", "--no-renames", base, fork).split("\n").filter(Boolean));
  const added = addedLinesByFile(git("diff", "-U0", "--no-renames", base, upstream));
  const rewritten = [];
  for (const [file, lines] of added) {
    if (forkTouched.has(file) || !TEXT.test(file) || SKIP.test(file)) continue;
    let text;
    try {
      text = readFileSync(join(root, file), "utf8");
    } catch {
      continue;
    }
    const r = rebrandText(text, lines);
    if (r.changed) {
      writeFileSync(join(root, file), r.text);
      rewritten.push({ file, lines: r.changed });
    }
  }
  return rewritten;
}
