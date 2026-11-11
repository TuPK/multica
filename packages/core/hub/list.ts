import type { PluginInstallation, PluginPackage } from "../types/plugin";
import type { HubItemMeta, HubItemTracking } from "../types/hub";
import { filterHubItems, sortHubItems, type HubItem, type HubItemKind, type HubSourceType } from "./items";
import { findHubItemMeta } from "./item-meta";

/** Status filters. Update and scan come from the item's sync metadata; tracking modes from the same. */
export type HubStatusFilter = "update_available" | "scan_warning" | "tracking" | "forked" | "manual";
export type HubUsageFilter = "used" | "unused";

export type HubListSortField = "name" | "updated" | "created" | "usedBy" | "source";

export const HUB_LIST_SORT_DEFAULT_DIRECTION: Record<HubListSortField, "asc" | "desc"> = {
  name: "asc",
  updated: "desc",
  created: "desc",
  usedBy: "desc",
  source: "asc",
};

/**
 * Every dimension is a multi-select. Values inside one dimension match when any of
 * them matches; dimensions combine with AND. An empty dimension does not filter.
 */
export interface HubListFilters {
  kinds: HubItemKind[];
  sources: HubSourceType[];
  labels: string[];
  statuses: HubStatusFilter[];
  usage: HubUsageFilter[];
}

export const EMPTY_HUB_LIST_FILTERS: HubListFilters = {
  kinds: [],
  sources: [],
  labels: [],
  statuses: [],
  usage: [],
};

/** A hub item joined with its sync metadata and usage. */
export interface HubListRow extends HubItem {
  /** Agents that use a skill, or workspace installations of a plugin. */
  usedBy: number;
  updateAvailable: boolean;
  scanWarning: boolean;
  /** Null when the item has no metadata row yet (it is created lazily on first read). */
  tracking: HubItemTracking | null;
}

/** Usage counts keyed by `HubItem.key`. */
export function buildHubUsage(
  skillAssignments: ReadonlyMap<string, readonly unknown[]>,
  packages: readonly PluginPackage[],
  installations: readonly PluginInstallation[],
): Map<string, number> {
  const usage = new Map<string, number>();
  for (const [skillId, agents] of skillAssignments) usage.set(`skill:${skillId}`, agents.length);
  for (const pkg of packages) {
    const count = installations.filter((installation) => installation.plugin_key === pkg.plugin_key).length;
    usage.set(`plugin:${pkg.id}`, count);
  }
  return usage;
}

export function buildHubListRows(
  items: readonly HubItem[],
  meta: readonly HubItemMeta[],
  usage: ReadonlyMap<string, number>,
): HubListRow[] {
  return items.map((item) => {
    const row = findHubItemMeta(meta, item.kind, item.refId);
    return {
      ...item,
      usedBy: usage.get(item.key) ?? 0,
      updateAvailable: row?.update_available ?? false,
      scanWarning: row ? row.scan_status !== "clean" : false,
      tracking: row?.tracking ?? null,
    };
  });
}

export function filterHubListRows(
  rows: readonly HubListRow[],
  filters: HubListFilters,
  query: string,
): HubListRow[] {
  // Text, kind and source come from the shared item filter, so both lists search the same fields.
  const base = new Set(
    filterHubItems(rows, { query, kinds: filters.kinds, sources: filters.sources }).map((row) => row.key),
  );
  return rows.filter((row) => {
    if (!base.has(row.key)) return false;
    if (filters.labels.length > 0 && !row.labels.some((label) => filters.labels.includes(label))) return false;
    if (filters.statuses.length > 0 && !filters.statuses.some((status) => statusMatches(row, status))) return false;
    if (filters.usage.length > 0 && !filters.usage.some((usage) => (usage === "used") === (row.usedBy > 0))) {
      return false;
    }
    return true;
  });
}

function statusMatches(row: HubListRow, status: HubStatusFilter): boolean {
  switch (status) {
    case "update_available":
      return row.updateAvailable;
    case "scan_warning":
      return row.scanWarning;
    default:
      return row.tracking === status;
  }
}

export function sortHubListRows(
  rows: readonly HubListRow[],
  field: HubListSortField,
  direction: "asc" | "desc",
): HubListRow[] {
  if (field === "usedBy" || field === "source") {
    const sign = direction === "asc" ? 1 : -1;
    return [...rows].sort((a, b) => {
      const cmp = field === "usedBy" ? a.usedBy - b.usedBy : a.source.localeCompare(b.source);
      return cmp !== 0 ? sign * cmp : a.name.localeCompare(b.name) || a.key.localeCompare(b.key);
    });
  }
  return sortHubItems(rows, field, direction);
}

/** Number of active filter values across every dimension, plus the search text. */
export function activeHubFilterCount(filters: HubListFilters, query: string): number {
  return (
    filters.kinds.length +
    filters.sources.length +
    filters.labels.length +
    filters.statuses.length +
    filters.usage.length +
    (query.trim() === "" ? 0 : 1)
  );
}

/** Distinct labels across the rows, sorted, for the label filter menu. */
export function hubListLabels(rows: readonly HubListRow[]): string[] {
  return [...new Set(rows.flatMap((row) => row.labels))].sort((a, b) => a.localeCompare(b));
}
