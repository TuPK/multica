#!/usr/bin/env node
// `pnpm sync:contract`: confirms the fork's own features still exist after a merge, using
// static checks listed in scripts/sync/fork-contract.json. It complements sync-audit.mjs
// (which finds lost lines) by asserting that named features are still there.
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const read = (file) => readFileSync(file, "utf8");

/** Runs one check; returns an error string or null. */
export function runCheck(root, check) {
  const path = join(root, check.path);
  switch (check.kind) {
    case "exists":
      return existsSync(path) ? null : "missing";
    case "absent":
      return existsSync(path) ? "present, but must not exist" : null;
    case "nonEmptyDir":
      return existsSync(path) && statSync(path).isDirectory() && readdirSync(path).length > 0 ? null : "missing or empty directory";
    case "matches":
      return existsSync(path) && new RegExp(check.pattern).test(read(path)) ? null : `file missing or pattern not found: ${check.pattern}`;
    case "lacks":
      return existsSync(path) && new RegExp(check.pattern).test(read(path)) ? `forbidden pattern found: ${check.pattern}` : null;
    case "jsonEquals": {
      if (!existsSync(path)) return "missing";
      const value = check.key.split(".").reduce((o, k) => o?.[k], JSON.parse(read(path)));
      return value === check.value ? null : `${check.key} is ${JSON.stringify(value)}, expected ${JSON.stringify(check.value)}`;
    }
    default:
      return `unknown check kind: ${check.kind}`;
  }
}

/** Returns [{feature, check, error}] for every failing check. */
export function runContract(root, contract) {
  const failures = [];
  for (const feature of contract.features) {
    for (const check of feature.checks) {
      const error = runCheck(root, check);
      if (error) failures.push({ feature: feature.id, why: feature.why, check, error });
    }
  }
  return failures;
}

export function main() {
  const here = dirname(fileURLToPath(import.meta.url));
  const root = join(here, "..", "..");
  const contract = JSON.parse(read(join(here, "fork-contract.json")));
  const failures = runContract(root, contract);
  const checks = contract.features.reduce((n, f) => n + f.checks.length, 0);
  if (!failures.length) {
    console.log(`sync:contract ok (${contract.features.length} features, ${checks} checks)`);
    return 0;
  }
  for (const f of failures) console.error(`FORK FEATURE LOST: ${f.feature}: ${f.check.kind} ${f.check.path}: ${f.error}\n  why it matters: ${f.why}`);
  console.error(`${failures.length} of ${checks} checks failed`);
  return 1;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) process.exit(main());
