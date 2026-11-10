import type { PluginPackage, SkillSummary } from "../types";

export type HubItemKind = "skill" | "plugin";

/** Where an item came from. `url_zip`, `registry` and `upload` are the add-flow sources. */
export type HubSourceType =
  | "manual"
  | "runtime_local"
  | "clawhub"
  | "skills_sh"
  | "github"
  | "plugin"
  | "url_zip"
  | "registry"
  | "upload";

/** One row of the Hub: a skill or a published plugin, seen through one shape. */
export interface HubItem {
  /** Unique across kinds: `${kind}:${refId}`. */
  key: string;
  kind: HubItemKind;
  refId: string;
  name: string;
  description: string;
  source: HubSourceType;
  /** Link to where the item came from, when the source has one. */
  sourceUrl: string | null;
  version: string | null;
  labels: string[];
  updatedAt: string;
  createdAt: string;
}

export type HubSortField = "name" | "updated" | "created";

export interface HubFilters {
  query: string;
  kinds: HubItemKind[];
  sources: HubSourceType[];
}

export const EMPTY_HUB_FILTERS: HubFilters = { query: "", kinds: [], sources: [] };

const KNOWN_SOURCE_TYPES = new Set<string>(["github", "skills_sh", "clawhub", "url_zip", "registry", "upload"]);

/** A source type from the server that this client does not know is shown as manual. */
export function normalizeHubSourceType(value: string): HubSourceType {
  return KNOWN_SOURCE_TYPES.has(value) ? (value as HubSourceType) : "manual";
}

const KNOWN_SKILL_ORIGINS = new Set<string>(["runtime_local", "clawhub", "skills_sh", "github"]);

function skillSource(skill: SkillSummary): { source: HubSourceType; sourceUrl: string | null } {
  const origin = skill.config?.origin as { type?: unknown; source_url?: unknown } | undefined;
  const type = typeof origin?.type === "string" ? origin.type : "manual";
  const source = KNOWN_SKILL_ORIGINS.has(type) ? (type as HubSourceType) : "manual";
  const url = typeof origin?.source_url === "string" ? origin.source_url : "";
  return { source, sourceUrl: /^https?:\/\//.test(url) ? url : null };
}

export function buildHubItems(
  skills: readonly SkillSummary[],
  packages: readonly PluginPackage[],
): HubItem[] {
  const items: HubItem[] = [];
  for (const skill of skills) {
    items.push({
      key: `skill:${skill.id}`,
      kind: "skill",
      refId: skill.id,
      name: skill.name,
      description: skill.description,
      ...skillSource(skill),
      version: null,
      labels: (skill.labels ?? []).map((label) => label.name),
      updatedAt: skill.updated_at,
      createdAt: skill.created_at,
    });
  }
  for (const pkg of packages) {
    const newest = pkg.versions[0];
    items.push({
      key: `plugin:${pkg.id}`,
      kind: "plugin",
      refId: pkg.id,
      name: pkg.name,
      description: pkg.plugin_key,
      source: "plugin",
      sourceUrl: null,
      version: newest?.version ?? null,
      labels: [],
      updatedAt: newest?.published_at ?? pkg.created_at,
      createdAt: pkg.created_at,
    });
  }
  return items;
}

export function filterHubItems(items: readonly HubItem[], filters: HubFilters): HubItem[] {
  const query = filters.query.trim().toLowerCase();
  return items.filter((item) => {
    if (filters.kinds.length > 0 && !filters.kinds.includes(item.kind)) return false;
    if (filters.sources.length > 0 && !filters.sources.includes(item.source)) return false;
    if (query === "") return true;
    return (
      item.name.toLowerCase().includes(query) ||
      item.description.toLowerCase().includes(query) ||
      item.labels.some((label) => label.toLowerCase().includes(query))
    );
  });
}

export function sortHubItems(
  items: readonly HubItem[],
  field: HubSortField,
  direction: "asc" | "desc",
): HubItem[] {
  const sign = direction === "asc" ? 1 : -1;
  return [...items].sort((a, b) => {
    const cmp =
      field === "name"
        ? a.name.localeCompare(b.name)
        : field === "updated"
          ? a.updatedAt.localeCompare(b.updatedAt)
          : a.createdAt.localeCompare(b.createdAt);
    return cmp !== 0 ? sign * cmp : a.key.localeCompare(b.key);
  });
}

export interface HubSourceSummary {
  source: HubSourceType;
  count: number;
  skills: number;
  plugins: number;
  lastUpdatedAt: string;
}

/** One row per source type present, derived from the items — real source entities arrive with the add flows. */
export function summarizeHubSources(items: readonly HubItem[]): HubSourceSummary[] {
  const bySource = new Map<HubSourceType, HubSourceSummary>();
  for (const item of items) {
    const entry = bySource.get(item.source) ?? {
      source: item.source,
      count: 0,
      skills: 0,
      plugins: 0,
      lastUpdatedAt: "",
    };
    entry.count += 1;
    if (item.kind === "skill") entry.skills += 1;
    else entry.plugins += 1;
    if (item.updatedAt > entry.lastUpdatedAt) entry.lastUpdatedAt = item.updatedAt;
    bySource.set(item.source, entry);
  }
  return [...bySource.values()].sort((a, b) => b.count - a.count || a.source.localeCompare(b.source));
}
