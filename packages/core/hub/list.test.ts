// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { HubItemMeta } from "../types/hub";
import type { PluginInstallation, PluginPackage } from "../types/plugin";
import { buildHubItems, type HubItem } from "./items";
import {
  activeHubFilterCount,
  buildHubListRows,
  buildHubUsage,
  EMPTY_HUB_LIST_FILTERS,
  filterHubListRows,
  hubListLabels,
  sortHubListRows,
  type HubListFilters,
} from "./list";

const skill = (id: string, name: string, extra: Record<string, unknown> = {}) => ({
  id,
  workspace_id: "ws",
  name,
  description: `${name} description`,
  config: extra.origin ? { origin: extra.origin } : {},
  created_by: null,
  created_at: String(extra.created ?? "2026-01-01T00:00:00Z"),
  updated_at: String(extra.updated ?? "2026-01-01T00:00:00Z"),
  labels: (extra.labels as string[] | undefined)?.map((name) => ({ id: name, name })) ?? [],
  content: "",
  files: [],
});

const pkg = (id: string, pluginKey: string, name: string): PluginPackage =>
  ({
    id,
    workspace_id: "ws",
    plugin_key: pluginKey,
    name,
    created_by: null,
    created_at: "2026-01-02T00:00:00Z",
    versions: [{ id: `${id}-v1`, version: "1.0.0", published_at: "2026-01-03T00:00:00Z" }],
  }) as unknown as PluginPackage;

function meta(kind: "skill" | "plugin", refId: string, patch: Partial<HubItemMeta> = {}): HubItemMeta {
  return {
    id: `hi-${refId}`,
    kind,
    ref_id: refId,
    source_id: null,
    source_ref: "",
    tracking: "manual",
    latest_version: "",
    bundle_digest: "",
    size_bytes: 0,
    file_count: 0,
    scan_status: "clean",
    update_available: false,
    remote_digest: "",
    checked_at: null,
    local_modified: false,
    created_at: "",
    updated_at: "",
    ...patch,
  };
}

const skills = [
  skill("s1", "Alpha", { origin: { type: "github", source_url: "https://github.com/a/alpha" }, labels: ["docs"], updated: "2026-03-01T00:00:00Z" }),
  skill("s2", "Beta", { labels: ["ops", "docs"], updated: "2026-01-01T00:00:00Z" }),
  skill("s3", "Gamma", { origin: { type: "clawhub" }, labels: ["ops"], updated: "2026-02-01T00:00:00Z" }),
];
const packages = [pkg("p1", "acme.deploy", "Deploy")];
const items: HubItem[] = buildHubItems(skills as never, packages);
const installations = [
  { id: "i1", plugin_key: "acme.deploy" } as PluginInstallation,
];
const usage = buildHubUsage(
  new Map([
    ["s1", [{}, {}, {}]],
    ["s2", [{}]],
  ]),
  packages,
  installations,
);
const metaRows = [
  meta("skill", "s1", { update_available: true }),
  meta("skill", "s2", { scan_status: "warn", tracking: "forked" }),
  meta("skill", "s3", { tracking: "tracking" }),
  meta("plugin", "p1", { tracking: "tracking" }),
];
const rows = buildHubListRows(items, metaRows, usage);

function keys(list: { refId: string }[]) {
  return list.map((row) => row.refId);
}

function filtered(patch: Partial<HubListFilters>, query = "") {
  return keys(filterHubListRows(rows, { ...EMPTY_HUB_LIST_FILTERS, ...patch }, query));
}

describe("buildHubUsage", () => {
  it("counts agents for skills and workspace installations for plugins", () => {
    expect(usage.get("skill:s1")).toBe(3);
    expect(usage.get("plugin:p1")).toBe(1);
    expect(usage.get("skill:s3")).toBeUndefined();
  });
});

describe("buildHubListRows", () => {
  it("joins sync metadata and usage onto each item, defaulting when there is no metadata", () => {
    const alpha = rows.find((row) => row.refId === "s1");
    expect(alpha).toMatchObject({ usedBy: 3, updateAvailable: true, scanWarning: false, tracking: "manual" });
    const beta = rows.find((row) => row.refId === "s2");
    expect(beta).toMatchObject({ scanWarning: true, tracking: "forked", usedBy: 1 });
    const bare = buildHubListRows(items, [], usage).find((row) => row.refId === "s3");
    expect(bare).toMatchObject({ tracking: null, updateAvailable: false, usedBy: 0 });
  });
});

describe("filterHubListRows", () => {
  it("returns everything when no dimension is set", () => {
    expect(filtered({})).toEqual(["s1", "s2", "s3", "p1"]);
  });

  it("filters by kind and by source, each multi-select", () => {
    expect(filtered({ kinds: ["plugin"] })).toEqual(["p1"]);
    expect(filtered({ sources: ["github", "clawhub"] })).toEqual(["s1", "s3"]);
  });

  it("matches labels with OR inside the dimension and excludes plugins, which have none", () => {
    expect(filtered({ labels: ["docs"] })).toEqual(["s1", "s2"]);
    expect(filtered({ labels: ["docs", "ops"] })).toEqual(["s1", "s2", "s3"]);
    expect(filtered({ labels: ["nope"] })).toEqual([]);
  });

  it("combines status values with OR and dimensions with AND", () => {
    expect(filtered({ statuses: ["update_available", "scan_warning"] })).toEqual(["s1", "s2"]);
    expect(filtered({ statuses: ["forked", "tracking"] })).toEqual(["s2", "s3", "p1"]);
    expect(filtered({ statuses: ["update_available"], labels: ["ops"] })).toEqual([]);
  });

  it("filters by usage: used means at least one agent or installation", () => {
    expect(filtered({ usage: ["used"] })).toEqual(["s1", "s2", "p1"]);
    expect(filtered({ usage: ["unused"] })).toEqual(["s3"]);
  });

  it("applies the search text over name, description and labels", () => {
    expect(filtered({}, "GAMMA")).toEqual(["s3"]);
    expect(filtered({}, "ops")).toEqual(["s2", "s3"]);
    expect(filtered({}, "clawhub")).toEqual(["s3"]);
    expect(filtered({ kinds: ["skill"] }, "deploy")).toEqual([]);
  });
});

describe("sortHubListRows", () => {
  it("sorts by usage, source, name and recency", () => {
    expect(keys(sortHubListRows(rows, "usedBy", "desc"))).toEqual(["s1", "s2", "p1", "s3"]);
    // clawhub < github < manual < plugin
    expect(keys(sortHubListRows(rows, "source", "asc"))).toEqual(["s3", "s1", "s2", "p1"]);
    expect(keys(sortHubListRows(rows, "name", "asc"))).toEqual(["s1", "s2", "p1", "s3"]);
    // Alpha (March) is newer than the plugin's January publish.
    expect(keys(sortHubListRows(rows, "updated", "desc"))).toEqual(["s1", "s3", "p1", "s2"]);
  });
});

describe("hubListLabels and activeHubFilterCount", () => {
  it("lists distinct labels sorted, and counts active filter values plus the search", () => {
    expect(hubListLabels(rows)).toEqual(["docs", "ops"]);
    expect(
      activeHubFilterCount({ ...EMPTY_HUB_LIST_FILTERS, kinds: ["skill"], labels: ["docs", "ops"] }, " beta "),
    ).toBe(4);
    expect(activeHubFilterCount(EMPTY_HUB_LIST_FILTERS, "   ")).toBe(0);
  });
});
