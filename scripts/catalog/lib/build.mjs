// Writes the generated code. Everything below tsOutDir and goOutDir is rebuilt from scratch each
// run, so a removed template never leaves a stale file behind.
import { execFileSync } from "node:child_process";
import { mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { catalogDir, repoRoot } from "./paths.mjs";
import { renderSkillsBuiltin, renderSkillsets } from "./build-skills.mjs";
import { buildPackBundle, packDirs, renderPackIndex } from "./build-packs.mjs";
import { renderCollections, renderI18n, renderRoleInstructions, renderSquads } from "./build-squads.mjs";
import { companyForGo, renderAgents, renderCompany } from "./build-company.mjs";
import { designerTemplates, flowsById, renderFlows, renderWorkflows, workflowsById } from "./build-workflows.mjs";
import { contractsIndex, renderContracts } from "./build-contracts.mjs";

function write(path, text) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, text);
  return 1;
}

export function buildAll({ entries, config, target, tsOutDir, goOutDir }) {
  let written = 0;
  if (target === "ts" || target === "all") {
    rmSync(tsOutDir, { recursive: true, force: true });
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
    execFileSync(process.execPath, [join(repoRoot, "scripts/generate-kitviet-pack.mjs"), "--out", join(tsOutDir, "kitviet")], { stdio: ["ignore", "ignore", "inherit"] });
    written += 29;
    const packIds = [];
    for (const dir of packDirs(catalogDir)) {
      const bundle = buildPackBundle(dir, repoRoot);
      packIds.push(bundle.id);
      written += write(join(tsOutDir, "agent-packs", `${bundle.id}.json`), `${JSON.stringify(bundle, null, 2)}\n`);
    }
    written += write(join(tsOutDir, "agent-packs", "index.ts"), renderPackIndex(packIds));
  }
  if (target === "go" || target === "all") {
    mkdirSync(goOutDir, { recursive: true });
    // Rebuilt from scratch; data/.gitkeep is tracked and stays.
    for (const file of readdirSync(goOutDir).filter((f) => f.endsWith(".json"))) rmSync(join(goOutDir, file));
    written += write(join(goOutDir, "company.json"), `${JSON.stringify(companyForGo(entries, catalogDir))}\n`);
    written += write(join(goOutDir, "workflows.json"), `${JSON.stringify(workflowsById(entries))}\n`);
    written += write(join(goOutDir, "flows.json"), `${JSON.stringify(flowsById(entries))}\n`);
    written += write(join(goOutDir, "designer-workflows.json"), `${JSON.stringify(designerTemplates(entries))}\n`);
    written += write(join(goOutDir, "contracts.json"), `${JSON.stringify(contractsIndex(entries))}\n`);
    const bundle = entries.find((e) => e.kind === "bundle" && e.manifest.id === "quickstart");
    if (!bundle) throw new Error("catalog/bundles/quickstart is missing");
    written += write(join(goOutDir, "quickstart-bundle.json"), `${JSON.stringify(bundle.manifest.bundle)}\n`);
  }
  return written;
}
