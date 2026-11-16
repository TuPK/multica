// `catalog map <domain>`: the one-page answer to "what does this domain contain". It reads the same
// manifests the build reads, so the map cannot drift from the data. Kit Viet squads are not copied into
// the catalog: their skills are read from catalog/_sources/kitviet/squads/*.json, as the generator does.
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { skillsetMembers } from "./build-skills.mjs";


const byOrder = (a, b) => (a.manifest.order ?? 1e9) - (b.manifest.order ?? 1e9) || (a.manifest.id < b.manifest.id ? -1 : 1);
const uniq = (values) => [...new Set(values)];

// The squad source files of the Kit Viet generator, by the squad id they build.
function kitvietSources(root) {
  const dir = join(root, "_sources", "kitviet", "squads");
  if (!existsSync(dir)) return new Map();
  const out = new Map();
  for (const name of readdirSync(dir).filter((f) => f.endsWith(".json"))) {
    const source = JSON.parse(readFileSync(join(dir, name), "utf8"));
    out.set(source.templateId, source);
  }
  return out;
}

// The skill names a Kit Viet squad installs: the shared ones, the leader's and each role's. Installed names
// carry the `kv:` prefix.
function kitvietSkills(source) {
  const names = [...(source.builtinSkills ?? []), ...(source.leaderSkills ?? []), ...Object.values(source.roleSkills ?? {}).flat()];
  return uniq(names).map((name) => `kv:${name}`);
}

function squadRow(squad, entries, kitviet, setMembers) {
  const { manifest } = squad;
  const kv = kitviet.get(manifest.id);
  const isKitviet = manifest.adapter?.type === "kitviet";
  const skills = isKitviet
    ? kv
      ? kitvietSkills(kv)
      : []
    : uniq([...(manifest.squad.skills?.sets ?? []).flatMap((set) => setMembers[set] ?? []), ...(manifest.squad.skills?.extra ?? []), ...(manifest.squad.skills?.leader ?? []), ...Object.values(manifest.squad.skills?.byRole ?? {}).flat()]);
  return {
    id: manifest.id,
    name: manifest.squad.name,
    collection: manifest.collection,
    order: manifest.order ?? null,
    markets: manifest.markets ?? [],
    kitviet: isKitviet,
    roles: manifest.squad.roles.length,
    skills: skills.length,
    flow: entries.some((e) => e.kind === "workflow" && e.manifest.id === `flow_${manifest.id}`),
    includes: manifest.includes ?? [],
  };
}

// The map of one domain. Throws when the domain does not exist.
export function buildMap(entries, domainId, root) {
  const domain = entries.find((e) => e.kind === "domain" && e.manifest.id === domainId);
  if (!domain) {
    const known = entries.filter((e) => e.kind === "domain").map((e) => e.manifest.id);
    throw new Error(`no domain "${domainId}"; known domains: ${known.join(", ") || "none"}`);
  }
  const kitviet = kitvietSources(root);
  const members = (kind) => entries.filter((e) => e.kind === kind && e.manifest.domain === domainId).sort(byOrder);
  const setMembers = skillsetMembers(entries);
  const squads = members("squad").map((s) => squadRow(s, entries, kitviet, setMembers));

  const skillEntries = members("skill");
  const groups = {};
  for (const skill of skillEntries) {
    // The group is shown below the domain's own folder (skills/<domain>/<group>), so "measurement", not "marketing/measurement".
    const group = (skill.group ?? "").replace(new RegExp(`^${domainId}/?`), "");
    (groups[group] ??= []).push({ id: skill.manifest.id, markets: skill.manifest.markets ?? [], source: skill.manifest.source?.type ?? "unknown" });
  }
  for (const list of Object.values(groups)) list.sort((a, b) => (a.id < b.id ? -1 : 1));

  const warnings = [];
  // A skill under skills/<domain>/ that names no domain is invisible to this map: say so.
  for (const skill of entries.filter((e) => e.kind === "skill" && (e.group ?? "").split("/")[0] === domainId && e.manifest.domain === undefined)) {
    warnings.push(`skill ${skill.manifest.id} sits under skills/${domainId}/ but has no "domain"`);
  }
  for (const squad of squads) {
    for (const included of squad.includes) if (!entries.some((e) => e.kind === "squad" && e.manifest.id === included)) warnings.push(`squad ${squad.id} includes "${included}", which does not exist`);
  }

  return {
    domain: { id: domainId, version: domain.manifest.version, markets: domain.manifest.markets },
    squads,
    skills: { total: skillEntries.length, groups },
    packs: members("pack").map((p) => {
      // A pack's agent and skill list live in pack.json; its template.json holds only the catalog fields.
      const pack = JSON.parse(readFileSync(join(p.dir, "pack.json"), "utf8"));
      return { id: p.manifest.id, name: pack.agent?.name ?? null, skills: pack.skills ?? [], markets: p.manifest.markets ?? [] };
    }),
    related: domain.manifest.related ?? [],
    warnings,
  };
}

// The text form: a short list per part, enough to see what is there. `--json` gives the full map.
export function formatMap(map) {
  const lines = [];
  const markets = map.domain.markets.map((m) => `${m.id} (${m.role})`).join(", ");
  lines.push(`Domain ${map.domain.id} ${map.domain.version}   markets: ${markets}`, "");
  lines.push(`Squads (${map.squads.length})`);
  for (const s of map.squads) {
    const tags = [s.kitviet ? "kitviet" : null, s.flow ? "flow" : null, s.markets.length ? s.markets.join(",") : null].filter(Boolean).join(" · ");
    lines.push(`  ${String(s.order ?? "").padStart(3)}  ${s.id.padEnd(16)} ${s.name}`);
    lines.push(`       collection ${s.collection} · ${s.roles} roles · ${s.skills} skills${tags ? ` · ${tags}` : ""}${s.includes.length ? ` · includes ${s.includes.join(", ")}` : ""}`);
  }
  lines.push("", `Skills (${map.skills.total})`);
  for (const [group, list] of Object.entries(map.skills.groups)) {
    lines.push(`  ${group || "(root)"} (${list.length}): ${list.map((s) => s.id).join(", ")}`);
  }
  if (map.packs.length) {
    lines.push("", `Packs (${map.packs.length})`);
    for (const p of map.packs) lines.push(`  ${p.id}: ${p.skills.length} skills`);
  }
  if (map.related.length) {
    lines.push("", "Related (outside the domain)");
    for (const r of map.related) lines.push(`  ${r.kind} ${r.id}${r.note ? `: ${r.note}` : ""}`);
  }
  if (map.warnings.length) {
    lines.push("", "Warnings");
    for (const w of map.warnings) lines.push(`  ${w}`);
  }
  return `${lines.join("\n")}\n`;
}

