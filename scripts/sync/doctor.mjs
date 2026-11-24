#!/usr/bin/env node
// `pnpm sync:doctor`: checks that the merge drivers named in .gitattributes are defined
// and point at scripts that exist. Exit 1 when anything is missing.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";

const git = (...args) => execFileSync("git", args, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();
const config = (key) => {
  try {
    return git("config", "--get", key);
  } catch {
    return "";
  }
};

/** Sample path per driver; `git check-attr` must resolve it to that driver. */
export const SAMPLES = {
  brand: ["README.md", "apps/docs/content/docs/agents.mdx", "server/internal/daemon/daemon.go"],
  jsonmerge: ["packages/views/locales/en/auth.json", "apps/docs/content/docs/meta.json", "package.json", "apps/desktop/package.json"],
  ours: ["pnpm-lock.yaml", "server/pkg/db/generated/models.go"],
};

export function check() {
  const problems = [];
  for (const [driver, samples] of Object.entries(SAMPLES)) {
    const cmd = config(`merge.${driver}.driver`);
    if (!cmd) problems.push(`merge.${driver}.driver is not set (run: bash scripts/sync/install.sh)`);
    for (const script of cmd.match(/[^\s"]+\.mjs/g) ?? []) {
      if (!existsSync(script)) problems.push(`merge.${driver}.driver points at a missing script: ${script}`);
    }
    for (const path of samples) {
      const out = git("check-attr", "merge", "--", path);
      if (!out.endsWith(`: ${driver}`)) problems.push(`.gitattributes does not map ${path} to ${driver} (got: ${out})`);
    }
  }
  if (config("rerere.enabled") !== "true") problems.push("rerere.enabled is not true");
  return problems;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const problems = check();
  if (problems.length) {
    console.error(problems.map((p) => `sync:doctor: ${p}`).join("\n"));
    process.exit(1);
  }
  console.log("sync:doctor: merge drivers installed and mapped");
}
