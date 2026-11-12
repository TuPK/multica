#!/usr/bin/env node
// The catalog CLI (docs/specs/template-registry/spec.md).
//
//   node scripts/catalog/cli.mjs check [--base <git ref>]   validate catalog/ (nothing is written)
//   node scripts/catalog/cli.mjs build [--target ts|go|all] validate, then write the generated code
//   node scripts/catalog/cli.mjs new <kind> <id> [options]   scaffold a template
//   node scripts/catalog/cli.mjs list                        print every template
//
// Generated code is not committed: this runs in every build entry point (turbo task //#catalog:build,
// make catalog-build, the Dockerfile stages). It needs only Node, no installed packages.
import { execFileSync } from "node:child_process";
import { relative } from "node:path";
import { pathToFileURL } from "node:url";
import { discover, readRegistryConfig } from "./lib/discover.mjs";
import { catalogDir, goOutDir, repoRoot, tsOutDir } from "./lib/paths.mjs";
import { validateCatalog } from "./lib/validate.mjs";
import { scaffold } from "./lib/scaffold.mjs";
import { buildAll } from "./lib/build.mjs";

export function load(root = catalogDir) {
  const config = readRegistryConfig(root);
  const { entries, errors } = discover(root);
  const problems = validateCatalog({ entries, discoverErrors: errors, config });
  return { config, entries, problems };
}

function report(problems, { quiet = false } = {}) {
  for (const warning of problems.warnings) if (!quiet) console.warn(`warning: ${warning}`);
  if (problems.errors.length > 0) {
    console.error(`catalog check failed (${problems.errors.length} problems):`);
    for (const error of problems.errors.slice(0, 80)) console.error(`  ${error}`);
    if (problems.errors.length > 80) console.error(`  ... and ${problems.errors.length - 80} more`);
    return false;
  }
  return true;
}

// L7: ids of the base ref must still exist (or be listed in registry.config.json "removed").
function baseIdProblems(base, entries, config) {
  const errors = [];
  let listing;
  try {
    listing = execFileSync("git", ["ls-tree", "-r", "--name-only", base, "--", "catalog"], { cwd: repoRoot, encoding: "utf8" });
  } catch {
    return [`cannot read git ref ${base} to compare ids`];
  }
  const current = new Set(entries.map((e) => `${e.kind}:${e.manifest?.id}`));
  const removed = new Set(config.removed ?? []);
  const KIND_BY_DIR = { collections: "collection", skillsets: "skillset", squads: "squad", workflows: "workflow", companies: "company", agents: "agent", packs: "pack", bundles: "bundle" };
  for (const line of listing.split("\n")) {
    const match = /^catalog\/([^/]+)\/([^/]+)\/template\.json$/.exec(line);
    if (match && KIND_BY_DIR[match[1]]) {
      const key = `${KIND_BY_DIR[match[1]]}:${match[2]}`;
      if (!current.has(key) && !removed.has(key)) errors.push(`${key} exists on ${base} but is gone; deprecate it or list it in registry.config.json "removed"`);
    }
  }
  return errors;
}

function flag(args, name) {
  const index = args.indexOf(name);
  return index === -1 ? undefined : args[index + 1];
}

export async function main(argv = process.argv.slice(2)) {
  const [command, ...args] = argv;
  switch (command) {
    case "check": {
      const { config, entries, problems } = load();
      const base = flag(args, "--base");
      if (base) problems.errors.push(...baseIdProblems(base, entries, config));
      if (!report(problems)) return 1;
      console.log(`catalog ok: ${entries.length} templates`);
      return 0;
    }
    case "build": {
      const target = flag(args, "--target") ?? "all";
      if (!["ts", "go", "all"].includes(target)) throw new Error(`--target must be ts, go or all, got ${target}`);
      const { config, entries, problems } = load();
      if (!report(problems, { quiet: true })) return 1;
      const written = buildAll({ entries, config, target, tsOutDir, goOutDir });
      console.log(`catalog build (${target}): ${entries.length} templates, ${written} files written`);
      return 0;
    }
    case "list": {
      const { entries } = load();
      for (const e of entries) console.log(`${e.kind.padEnd(10)} ${String(e.manifest?.id).padEnd(40)} ${e.manifest?.version ?? "?"}  ${relative(catalogDir, e.dir)}`);
      return 0;
    }
    case "new": {
      const [kind, id] = args;
      if (!kind || !id) throw new Error("usage: new <kind> <id> [--collection <id>] [--set <id>]");
      const created = scaffold({ kind, id, collection: flag(args, "--collection"), set: flag(args, "--set") });
      for (const file of created) console.log(`created ${relative(repoRoot, file)}`);
      return 0;
    }
    default:
      console.error("usage: cli.mjs <check|build|new|list>");
      return 2;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().then(
    (code) => process.exit(code),
    (err) => {
      console.error(err.stack ?? err.message);
      process.exit(1);
    },
  );
}
