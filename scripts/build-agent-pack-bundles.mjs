#!/usr/bin/env node
// Regenerates packages/core/agent-packs/bundles/<id>.json from every pack under
// examples/agent-packs/. The web and desktop apps install packs from these
// bundles; scripts/install-agent-pack.sh reads the pack directories directly.
import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { basename, dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const packsDir = join(root, "examples", "agent-packs");
const bundlesDir = join(root, "packages", "core", "agent-packs", "bundles");

function listFiles(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? listFiles(path) : [path];
  });
}

function readText(path) {
  const buffer = readFileSync(path);
  if (buffer.includes(0)) throw new Error(`${relative(root, path)} looks binary; packs carry text only`);
  return buffer.toString("utf8");
}

function buildBundle(packDir) {
  const id = basename(packDir);
  const manifest = JSON.parse(readText(join(packDir, "pack.json")));
  const { agent } = manifest;
  return {
    id,
    agent: {
      name: agent.name,
      description: agent.description,
      instructions: readText(join(packDir, agent.instructions_file)),
      max_concurrent_tasks: agent.max_concurrent_tasks,
    },
    skills: manifest.skills.map((name) => {
      const skillDir = join(packDir, "skills", name);
      return {
        name,
        files: listFiles(skillDir)
          .map((file) => ({
            path: relative(skillDir, file).split("\\").join("/"),
            content: readText(file),
          }))
          .sort((a, b) => a.path.localeCompare(b.path)),
      };
    }),
  };
}

mkdirSync(bundlesDir, { recursive: true });
for (const name of readdirSync(packsDir)) {
  const packDir = join(packsDir, name);
  if (!statSync(join(packDir, "pack.json"), { throwIfNoEntry: false })) continue;
  const bundle = buildBundle(packDir);
  writeFileSync(join(bundlesDir, `${bundle.id}.json`), `${JSON.stringify(bundle, null, 2)}\n`);
  console.log(`wrote packages/core/agent-packs/bundles/${bundle.id}.json (${bundle.skills.length} skills)`);
}
