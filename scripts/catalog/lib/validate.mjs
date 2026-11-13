// Validation of catalog manifests and content (spec layers L1 to L4 and L7). No dependencies:
// the build must run in a bare Node image (Dockerfile stage) without a pnpm install.
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { frontmatterDescription } from "../../lib/skill-bundle.mjs";
import { KIND_DIRS, skillIdOfFolder } from "./discover.mjs";
import { blocksOf, validateAgent, validateCompany } from "./build-company.mjs";
import { validateContracts } from "./build-contracts.mjs";
import { validatePack } from "./build-packs.mjs";
import { validateWorkflow } from "./build-workflows.mjs";
import { catalogDir, supportedLocalesFile } from "./paths.mjs";

const SEMVER = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
const STATUSES = ["stable", "beta", "deprecated", "hidden"];
const ID_RULES = {
  skill: /^[a-z0-9][a-z0-9._-]*(?::[a-z0-9][a-z0-9._-]*)?$/,
  pack: /^[a-z][a-z0-9-]{1,47}$/,
  default: /^[a-z][a-z0-9_]{1,47}$/,
};
// Kinds whose title and description appear in the product, so every locale must carry them.
export const KINDS_WITH_UI_TEXT = ["collection", "squad", "pack", "workflow"];

export { isNonEmptyString, isObject } from "./validate-shared.mjs";
import { isNonEmptyString, isObject } from "./validate-shared.mjs";

// SUPPORTED_LOCALES of packages/core/i18n/types.ts, read as text so this script needs no TypeScript.
export function supportedLocales() {
  const source = readFileSync(supportedLocalesFile, "utf8");
  const block = /SUPPORTED_LOCALES: SupportedLocale\[\] = \[([^\]]*)\]/.exec(source)?.[1];
  if (!block) throw new Error(`cannot read SUPPORTED_LOCALES from ${supportedLocalesFile}`);
  return [...block.matchAll(/"([^"]+)"/g)].map((match) => match[1]);
}

export function skillFrontmatter(content) {
  const lines = content.split("\n");
  if (lines[0]?.trim() !== "---") return {};
  const end = lines.indexOf("---", 1);
  const block = lines.slice(1, end === -1 ? undefined : end);
  const nameLine = block.find((line) => line.startsWith("name:"));
  const name = nameLine?.slice("name:".length).trim().replace(/^["']|["']$/g, "");
  return { name };
}

// Each `kind:id` of the catalog, to resolve references.
const keyOf = (kind, id) => `${kind}:${id}`;

export function validateEnvelope(entry, problems) {
  const { manifest, kind, folder, path } = entry;
  const err = (message) => problems.errors.push(`${path}: ${message}`);
  if (!isObject(manifest)) return err("manifest must be a JSON object");
  if (manifest.schemaVersion !== 1) err(`schemaVersion must be 1, got ${JSON.stringify(manifest.schemaVersion)}`);
  if (manifest.kind !== kind) err(`kind must be "${kind}" (the folder it sits in), got ${JSON.stringify(manifest.kind)}`);
  const idRule = ID_RULES[kind] ?? ID_RULES.default;
  if (!isNonEmptyString(manifest.id) || !idRule.test(manifest.id)) {
    err(`id ${JSON.stringify(manifest.id)} does not match ${idRule}`);
  } else if (kind === "skill" ? skillIdOfFolder(folder) !== manifest.id : folder !== manifest.id) {
    err(`id "${manifest.id}" must equal the folder name "${folder}"${kind === "skill" ? " (a ':' is stored as '--')" : ""}`);
  }
  if (!isNonEmptyString(manifest.version) || !SEMVER.test(manifest.version)) err(`version must be semver, got ${JSON.stringify(manifest.version)}`);
  if (manifest.status !== undefined && !STATUSES.includes(manifest.status)) err(`status must be one of ${STATUSES.join(", ")}`);
  if (manifest.order !== undefined && typeof manifest.order !== "number") err("order must be a number");
  for (const field of ["tags", "contracts", "aliases"]) {
    if (manifest[field] !== undefined && !(Array.isArray(manifest[field]) && manifest[field].every(isNonEmptyString))) {
      err(`${field} must be an array of strings`);
    }
  }
  validateContracts(entry, problems);
  if (manifest.requires !== undefined) {
    const ok = Array.isArray(manifest.requires) && manifest.requires.every((item) => isObject(item) && isNonEmptyString(item.kind) && isNonEmptyString(item.id));
    if (!ok) err("requires must be an array of { kind, id }");
  }
  if (manifest.source !== undefined) {
    const source = manifest.source;
    if (!isObject(source) || !["authored", "vendored"].includes(source.type)) err('source.type must be "authored" or "vendored"');
  }
}

function listTextFiles(dir) {
  const out = [];
  const walk = (current) => {
    for (const name of readdirSync(current)) {
      const path = join(current, name);
      if (statSync(path).isDirectory()) walk(path);
      else out.push(path);
    }
  };
  walk(dir);
  return out;
}

// The secret-looking text we refuse to ship in a template. Deliberately narrow: a wide net flags the
// example tokens that vendored skills print on purpose.
const SECRET_PATTERNS = [
  [/AKIA[0-9A-Z]{16}/, "an AWS access key id"],
  [/-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----/, "a private key"],
  [/\bsk-ant-[A-Za-z0-9_-]{20,}/, "an Anthropic API key"],
  [/\bghp_[A-Za-z0-9]{36}\b/, "a GitHub token"],
];

function validateSkill(entry, problems, context) {
  const { manifest, dir, path } = entry;
  const err = (message) => problems.errors.push(`${path}: ${message}`);
  const content = readFileSync(join(dir, "SKILL.md"), "utf8");
  const { name } = skillFrontmatter(content);
  const after = manifest.id?.includes(":") ? manifest.id.split(":").pop() : manifest.id;
  if (name !== manifest.id && name !== after) err(`SKILL.md frontmatter name ${JSON.stringify(name)} must equal the id or the part after ':'`);
  try {
    if (!frontmatterDescription(content, entry.folder)) err("SKILL.md has an empty description");
  } catch (e) {
    err(e.message);
  }
  if (!Array.isArray(manifest.sets) || manifest.sets.length === 0 || !manifest.sets.every(isNonEmptyString)) {
    err("sets must list at least one skillset");
  } else {
    for (const set of manifest.sets) {
      if (!context.ids.has(keyOf("skillset", set))) err(`sets names unknown skillset "${set}"`);
    }
  }
  if (manifest.source?.type === "vendored") {
    for (const field of ["repo", "ref", "license"]) {
      if (!isNonEmptyString(manifest.source[field])) err(`vendored source needs ${field}`);
      else if (manifest.source[field] === "TODO") err(`vendored source ${field} is still TODO`);
    }
    // Not errors, but a person should look: no pin, no licence file upstream, or a licence with limits.
    if (manifest.source.ref === "unpinned") context.unpinned.add(manifest.id);
    if (manifest.source.license === "none-detected") context.noLicense.add(manifest.source.repo);
    if (String(manifest.source.license).startsWith("LicenseRef-")) context.restricted.add(`${manifest.id} (${manifest.source.license})`);
  }
  for (const file of listTextFiles(dir)) {
    const buffer = readFileSync(file);
    if (buffer.includes(0)) {
      err(`${relative(dir, file)} looks binary; skills carry text only`);
      continue;
    }
    const text = buffer.toString("utf8");
    for (const [pattern, label] of SECRET_PATTERNS) {
      if (pattern.test(text)) err(`${relative(dir, file)} contains ${label}`);
    }
  }
}

function validateSkillset(entry, problems) {
  const { manifest, path } = entry;
  if (!["quickstart", "starter"].includes(manifest.target)) problems.errors.push(`${path}: target must be "quickstart" or "starter"`);
}

// The i18n files of a template: { title, description } for every locale, nothing else, no blank value.
// A collection heading is a title alone; every other template shows a title and a description.
const I18N_KEYS = { collection: ["title"] };

export function validateI18n(entry, locales, problems) {
  const required = I18N_KEYS[entry.kind] ?? ["description", "title"];
  const dir = join(entry.dir, "i18n");
  const err = (message) => problems.errors.push(`${entry.path}: ${message}`);
  if (!existsSync(dir)) return err(`i18n/ is missing; every locale (${locales.join(", ")}) is required`);
  const present = readdirSync(dir).filter((file) => file.endsWith(".json")).map((file) => file.replace(/\.json$/, ""));
  for (const locale of locales) {
    if (!present.includes(locale)) {
      err(`i18n/${locale}.json is missing (no fallback to another language)`);
      continue;
    }
    let data;
    try {
      data = JSON.parse(readFileSync(join(dir, `${locale}.json`), "utf8"));
    } catch (e) {
      err(`i18n/${locale}.json is not valid JSON (${e.message})`);
      continue;
    }
    const keys = isObject(data) ? Object.keys(data).sort().join(",") : "";
    const want = [...required].sort().join(",");
    if (keys !== want) err(`i18n/${locale}.json must hold exactly ${required.map((k) => `"${k}"`).join(" and ")}, has: ${keys || "nothing"}`);
    else if (!required.every((key) => isNonEmptyString(data[key]))) err(`i18n/${locale}.json has an empty ${required.join(" or ")}`);
  }
  for (const locale of present) if (!locales.includes(locale)) err(`i18n/${locale}.json is not a supported locale`);
}

function validateBundle(entry, problems) {
  const bundle = entry.manifest.bundle;
  const err = (message) => problems.errors.push(`${entry.path}: ${message}`);
  if (!isObject(bundle)) return err("bundle block is missing");
  for (const field of ["marketplaces", "plugins", "skills"]) if (!Array.isArray(bundle[field])) err(`bundle.${field} must be an array`);
  if (!isObject(bundle.claude_code) || !isNonEmptyString(bundle.claude_code.npm_package)) err("bundle.claude_code.npm_package is required");
}

function validateCollection(entry, problems) {
  if (entry.manifest.for !== "squad") problems.errors.push(`${entry.path}: for must be "squad"`);
}

function validateSquad(entry, problems, context) {
  const { manifest, path } = entry;
  const err = (message) => problems.errors.push(`${path}: ${message}`);
  if (!context.ids.has(keyOf("collection", manifest.collection))) err(`collection "${manifest.collection}" does not exist`);
  if (manifest.expertReview !== undefined && typeof manifest.expertReview !== "boolean") err("expertReview must be a boolean");
  const squad = manifest.squad;
  if (!isObject(squad)) return err("squad block is missing");
  for (const field of ["name", "description"]) if (!isNonEmptyString(squad[field])) err(`squad.${field} must be a non-empty string`);
  if (squad.floorName !== null && !isNonEmptyString(squad.floorName)) err("squad.floorName must be a string or null");
  if (squad.workflow !== null) {
    const target = context.entries.find((e) => e.kind === "workflow" && e.manifest?.id === squad.workflow);
    if (!target || target.manifest.workflow === undefined) err(`squad.workflow "${squad.workflow}" is not an executable workflow in the catalog`);
  }
  if (squad.operatingReview !== undefined && typeof squad.operatingReview !== "boolean") err("squad.operatingReview must be a boolean");
  if (!Array.isArray(squad.routines)) err("squad.routines must be an array");
  if (!isObject(squad.charter)) err("squad.charter must be an object");
  const kitviet = manifest.adapter?.type === "kitviet";
  if (manifest.adapter !== undefined && !kitviet) err('adapter.type must be "kitviet"');
  if (!Array.isArray(squad.roles) || squad.roles.length === 0) return err("squad.roles must list at least one role");
  const keys = new Set();
  for (const role of squad.roles) {
    if (!isObject(role) || !isNonEmptyString(role.key)) { err("every role needs a key"); continue; }
    if (keys.has(role.key)) err(`role ${role.key} is listed twice`);
    keys.add(role.key);
    const needed = kitviet ? ["description", "role"] : ["agentTemplate", "name", "description", "role"];
    for (const field of needed) if (!isNonEmptyString(role[field])) err(`role ${role.key}: ${field} must be a non-empty string`);
    if (kitviet) for (const field of ["agentTemplate", "name"]) if (role[field] !== undefined) err(`role ${role.key}: ${field} comes from the Kit Viet pack, not the manifest`);
    // A role used by several squads (the SDLC squads share theirs) must be defined the same way everywhere.
    const seen = context.roleDefinitions.get(role.key);
    if (seen && JSON.stringify(seen.role) !== JSON.stringify(role)) err(`role ${role.key} is defined differently in ${seen.path}`);
    else if (!seen) context.roleDefinitions.set(role.key, { role, path });
  }
  if (!keys.has(squad.leader)) err(`squad.leader "${squad.leader}" is not one of its roles`);
  if (kitviet) {
    if (squad.skills !== undefined) err("squad.skills comes from the Kit Viet pack for a Kit Viet squad");
  } else {
    const skills = squad.skills;
    if (!isObject(skills) || !Array.isArray(skills.sets)) err("squad.skills.sets must be an array of skillset ids");
    else for (const set of skills.sets) if (!context.ids.has(keyOf("skillset", set))) err(`squad.skills names unknown skillset "${set}"`);
    if (!existsSync(join(entry.dir, "instructions.md"))) err("instructions.md is missing");
  }
  if (kitviet && existsSync(join(entry.dir, "instructions.md"))) err("a Kit Viet squad's instructions come from the pack; remove instructions.md");
  const rolesDir = join(entry.dir, "roles");
  if (existsSync(rolesDir)) {
    if (kitviet) err("a Kit Viet squad's role instructions come from the pack; remove roles/");
    for (const file of readdirSync(rolesDir)) {
      const key = file.replace(/\.md$/, "");
      if (!file.endsWith(".md") || !keys.has(key)) { err(`roles/${file} is not the instructions of one of the squad's roles`); continue; }
      if (readFileSync(join(rolesDir, file), "utf8").trim() === "") err(`roles/${file} is empty`);
      const first = context.roleInstructionFiles.get(key);
      if (first && first !== path) err(`role ${key} has instructions in ${first} too; keep them in one squad`);
      else context.roleInstructionFiles.set(key, path);
    }
  }
}

// The validators of the other kinds register themselves here as the catalog grows.
export const kindValidators = { skill: validateSkill, skillset: validateSkillset, collection: validateCollection, squad: validateSquad, agent: validateAgent, company: validateCompany, bundle: validateBundle, workflow: validateWorkflow, pack: validatePack };

export function validateCatalog({ entries, discoverErrors = [], config, root = catalogDir }) {
  const problems = { errors: [...discoverErrors], warnings: [] };
  const locales = supportedLocales();
  if (JSON.stringify(config.locales) !== JSON.stringify(locales)) {
    problems.errors.push(`registry.config.json locales ${JSON.stringify(config.locales)} must equal SUPPORTED_LOCALES ${JSON.stringify(locales)}`);
  }

  const ids = new Set();
  for (const entry of entries) {
    validateEnvelope(entry, problems);
    const key = keyOf(entry.kind, entry.manifest?.id);
    if (ids.has(key)) problems.errors.push(`${entry.path}: duplicate ${entry.kind} id "${entry.manifest?.id}"`);
    ids.add(key);
  }
  const removed = new Set(config.removed ?? []);
  for (const entry of entries) {
    if (removed.has(keyOf(entry.kind, entry.manifest?.id))) problems.errors.push(`${entry.path}: id was removed (registry.config.json) and cannot be reused`);
  }

  const context = { ids, entries, config, locales, unpinned: new Set(), noLicense: new Set(), restricted: new Set(), roleDefinitions: new Map(), roleInstructionFiles: new Map(), flowSquads: new Map(), blocks: blocksOf(root) };
  for (const entry of entries) {
    for (const required of entry.manifest?.requires ?? []) {
      if (!ids.has(keyOf(required.kind, required.id))) problems.errors.push(`${entry.path}: requires unknown ${required.kind} "${required.id}"`);
    }
    kindValidators[entry.kind]?.(entry, problems, context);
    if (KINDS_WITH_UI_TEXT.includes(entry.kind) && !(entry.kind === "workflow" && entry.manifest?.ui === false)) {
      validateI18n(entry, locales, problems);
    }
  }

  // Skills that no skillset claims, and skillsets that no skill claims.
  const used = new Set(entries.filter((e) => e.kind === "skill").flatMap((e) => e.manifest.sets ?? []));
  for (const entry of entries.filter((e) => e.kind === "skillset")) {
    if (!used.has(entry.manifest.id)) problems.warnings.push(`${entry.path}: no skill belongs to this skillset`);
  }
  if (context.unpinned.size > 0) problems.warnings.push(`${context.unpinned.size} vendored skills name no commit or version (ref "unpinned"): ${[...context.unpinned].slice(0, 4).join(", ")}...`);
  if (context.noLicense.size > 0) problems.warnings.push(`upstream repos with no licence file (check before shipping): ${[...context.noLicense].join(", ")}`);
  if (context.restricted.size > 0) problems.warnings.push(`vendored skills under a licence with limits: ${[...context.restricted].join(", ")}`);
  return { ...problems, context };
}

export { catalogDir };
