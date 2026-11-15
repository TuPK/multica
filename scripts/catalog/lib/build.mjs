// Writes the generated code. The output is replaced in place, never emptied first: `next dev` and `go test`
// read these directories while a build runs (turbo dev, `make test` and agents' go test all start one), and a
// file that is deleted and rewritten fails the reader with ENOENT. A file is rewritten only when its content
// changed, through a rename, so a no-op build leaves the watchers nothing to recompile. A file that no
// template produces any more is removed at the end, so a removed template never leaves a stale file behind.
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { catalogDir, repoRoot } from "./paths.mjs";
import { renderSkillsBuiltin, renderSkillsets } from "./build-skills.mjs";
import { buildPackBundle, packDirs, renderPackIndex } from "./build-packs.mjs";
import { renderCollections, renderI18n, renderRoleInstructions, renderSquads } from "./build-squads.mjs";
import { companyForGo, renderAgents, renderCompany } from "./build-company.mjs";
import { designerTemplates, flowsById, renderFlows, renderWorkflows, workflowsById } from "./build-workflows.mjs";
import { contractsIndex, renderContracts } from "./build-contracts.mjs";

// The files this build produced, so the stale ones can be removed afterwards.
let produced = new Set();

// Returns 1 when the file was written, 0 when it already had this content.
function write(path, text) {
  produced.add(resolve(path));
  mkdirSync(dirname(path), { recursive: true });
  try {
    if (readFileSync(path, "utf8") === text) return 0;
  } catch {
    // Missing: write it.
  }
  const temp = `${path}.${process.pid}.tmp`;
  writeFileSync(temp, text);
  renameSync(temp, path);
  return 1;
}

// Removes every file below dir that this build did not produce and that keep() does not claim.
function removeStale(dir, keep) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!keep(path, true)) {
        removeStale(path, keep);
        if (readdirSync(path).length === 0) rmSync(path, { recursive: true, force: true });
      }
    } else if (!produced.has(resolve(path)) && !keep(path, false)) {
      rmSync(path, { force: true });
    }
  }
}

export function buildAll({ entries, config, target, tsOutDir, goOutDir }) {
  let written = 0;
  produced = new Set();
  if (target === "ts" || target === "all") {
    written += write(join(tsOutDir, "skills-builtin.ts"), renderSkillsBuiltin(entries));
    written += write(join(tsOutDir, "skillsets.ts"), renderSkillsets(entries));
    written += write(join(tsOutDir, "collections.ts"), renderCollections(entries));
    written += write(join(tsOutDir, "squads.ts"), renderSquads(entries));
    written += write(join(tsOutDir, "i18n.ts"), renderI18n(entries, config.locales));
    written += write(join(tsOutDir, "role-instructions.ts"), renderRoleInstructions(entries));
    written += write(join(tsOutDir, "agents.ts"), renderAgents(entries, catalogDir));
    written += write(join(tsOutDir, "company.ts"), renderCompany(entries));
    written += write(join(tsOutDir, "workflows.ts"), renderWorkflows(entries));
    written += write(join(tsOutDir, "flows.ts"), renderFlows(entries));
    written += write(join(tsOutDir, "contracts.ts"), renderContracts(entries));
    // The kitviet folder is written (and its own stale files removed) by the generator.
    execFileSync(process.execPath, [join(repoRoot, "scripts/generate-kitviet-pack.mjs"), "--out", join(tsOutDir, "kitviet")], { stdio: ["ignore", "ignore", "inherit"] });
    const packIds = [];
    for (const dir of packDirs(catalogDir)) {
      const bundle = buildPackBundle(dir, repoRoot);
      packIds.push(bundle.id);
      written += write(join(tsOutDir, "agent-packs", `${bundle.id}.json`), `${JSON.stringify(bundle, null, 2)}\n`);
    }
    written += write(join(tsOutDir, "agent-packs", "index.ts"), renderPackIndex(packIds));
    const kitvietDir = join(tsOutDir, "kitviet");
    removeStale(tsOutDir, (path, isDir) => isDir && path === kitvietDir);
  }
  if (target === "go" || target === "all") {
    mkdirSync(goOutDir, { recursive: true });
    written += write(join(goOutDir, "company.json"), `${JSON.stringify(companyForGo(entries, catalogDir))}\n`);
    written += write(join(goOutDir, "workflows.json"), `${JSON.stringify(workflowsById(entries))}\n`);
    written += write(join(goOutDir, "flows.json"), `${JSON.stringify(flowsById(entries))}\n`);
    written += write(join(goOutDir, "designer-workflows.json"), `${JSON.stringify(designerTemplates(entries))}\n`);
    written += write(join(goOutDir, "contracts.json"), `${JSON.stringify(contractsIndex(entries))}\n`);
    const bundle = entries.find((e) => e.kind === "bundle" && e.manifest.id === "quickstart");
    if (!bundle) throw new Error("catalog/bundles/quickstart is missing");
    written += write(join(goOutDir, "quickstart-bundle.json"), `${JSON.stringify(bundle.manifest.bundle)}\n`);
    // .gitkeep is tracked and stays.
    removeStale(goOutDir, (path) => path.endsWith(".gitkeep"));
  }
  return written;
}
