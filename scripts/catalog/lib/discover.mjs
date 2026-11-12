// Finds every template under catalog/ and reads its manifest. A template is a folder with a
// template.json. Under skills/, a folder holding SKILL.md is a skill and the walk stops there, so a
// SKILL.md sample inside references/ is never taken for a skill.
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { catalogDir } from "./paths.mjs";

// kind -> the folder under catalog/ that holds it.
export const KIND_DIRS = {
  collection: "collections",
  skillset: "skillsets",
  skill: "skills",
  squad: "squads",
  workflow: "workflows",
  company: "companies",
  agent: "agents",
  pack: "packs",
  bundle: "bundles",
};

const readJson = (path) => JSON.parse(readFileSync(path, "utf8"));

export function readRegistryConfig(root = catalogDir) {
  return readJson(join(root, "registry.config.json"));
}

function subdirs(dir) {
  return existsSync(dir)
    ? readdirSync(dir, { withFileTypes: true }).filter((entry) => entry.isDirectory()).map((entry) => entry.name).sort()
    : [];
}

// Every skill folder under catalog/skills/<group>/<folder>. Returns { group, folder, dir }.
export function skillFolders(root = catalogDir) {
  const out = [];
  const walk = (dir, group) => {
    for (const name of subdirs(dir)) {
      const path = join(dir, name);
      if (existsSync(join(path, "SKILL.md"))) out.push({ group, folder: name, dir: path });
      else walk(path, group ? `${group}/${name}` : name);
    }
  };
  walk(join(root, KIND_DIRS.skill), "");
  return out;
}

// Reads every template.json. Problems that stop a template from being read are returned as errors.
export function discover(root = catalogDir) {
  const entries = [];
  const errors = [];
  const add = (kind, dir, folder) => {
    const manifestPath = join(dir, "template.json");
    if (!existsSync(manifestPath)) return false;
    try {
      entries.push({ kind, folder, dir, path: relative(root, manifestPath).split(sep).join("/"), manifest: readJson(manifestPath) });
    } catch (err) {
      errors.push(`${relative(root, manifestPath)}: not valid JSON (${err.message})`);
    }
    return true;
  };

  for (const [kind, dirName] of Object.entries(KIND_DIRS)) {
    if (kind === "skill") continue;
    for (const folder of subdirs(join(root, dirName))) add(kind, join(root, dirName, folder), folder);
  }
  for (const skill of skillFolders(root)) {
    if (!add("skill", skill.dir, skill.folder)) {
      errors.push(`${relative(root, skill.dir)}: has SKILL.md but no template.json`);
    } else {
      entries[entries.length - 1].group = skill.group;
    }
  }
  return { entries, errors };
}

export function isDirectory(path) {
  return existsSync(path) && statSync(path).isDirectory();
}

export const skillIdOfFolder = (folder) => folder.replaceAll("--", ":");
