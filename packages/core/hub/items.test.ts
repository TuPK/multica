// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { PluginPackage, SkillSummary } from "../types";
import {
  buildHubItems,
  EMPTY_HUB_FILTERS,
  filterHubItems,
  sortHubItems,
  summarizeHubSources,
} from "./items";

const skill = (over: Partial<SkillSummary> & { id: string; name: string }): SkillSummary =>
  ({
    workspace_id: "w",
    description: "",
    config: {},
    created_by: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
    ...over,
  }) as SkillSummary;

const pkg = (over: Partial<PluginPackage> & { id: string; name: string }): PluginPackage => ({
  plugin_key: "acme.tool",
  versions: [],
  created_at: "2026-02-01T00:00:00Z",
  ...over,
});

describe("buildHubItems", () => {
  it("maps skill origins, falling back to manual for unknown or absent origins", () => {
    const items = buildHubItems(
      [
        skill({ id: "1", name: "a", config: { origin: { type: "github", source_url: "https://github.com/x/y" } } }),
        skill({ id: "2", name: "b", config: { origin: { type: "weird" } } }),
        skill({ id: "3", name: "c", config: { origin: { type: "clawhub", source_url: "javascript:alert(1)" } } }),
        skill({ id: "4", name: "d" }),
      ],
      [],
    );
    expect(items.map((i) => i.source)).toEqual(["github", "manual", "clawhub", "manual"]);
    expect(items[0]!.sourceUrl).toBe("https://github.com/x/y");
    expect(items[2]!.sourceUrl).toBeNull();
  });

  it("maps plugin packages with their newest version", () => {
    const [item] = buildHubItems(
      [],
      [
        pkg({
          id: "p1",
          name: "Pulse",
          versions: [{ id: "v2", version: "2.0.0", digest: "", size_bytes: 1, published_at: "2026-03-01T00:00:00Z", installed: true }],
        }),
      ],
    );
    expect(item).toMatchObject({ key: "plugin:p1", kind: "plugin", source: "plugin", version: "2.0.0", updatedAt: "2026-03-01T00:00:00Z" });
  });

  it("keeps keys unique when a skill and a plugin share an id", () => {
    const items = buildHubItems([skill({ id: "same", name: "s" })], [pkg({ id: "same", name: "p" })]);
    expect(new Set(items.map((i) => i.key)).size).toBe(2);
  });
});

describe("skill origins map to their source", () => {
  it("keeps every origin the server records, not only the first four", () => {
    const origins = ["upload", "url_zip", "registry", "runtime_local", "skills_sh", "clawhub", "github"];
    const items = buildHubItems(
      origins.map((type, index) => ({
        id: `s${index}`,
        name: type,
        description: "",
        config: { origin: { type, source_url: "" } },
        labels: [],
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      })) as unknown as SkillSummary[],
      [],
    );
    expect(items.map((item) => item.source)).toEqual(origins);
  });

  it("shows an origin it does not know as created here", () => {
    const [item] = buildHubItems(
      [{ id: "s1", name: "x", description: "", config: { origin: { type: "future_source" } }, labels: [], created_at: "", updated_at: "" } as unknown as SkillSummary],
      [],
    );
    expect(item?.source).toBe("manual");
  });
});

describe("filter, sort and summarize", () => {
  const items = buildHubItems(
    [
      skill({ id: "1", name: "Deploy", description: "ship it", config: { origin: { type: "github" } } }),
      skill({ id: "2", name: "alpha", updated_at: "2026-05-01T00:00:00Z" }),
    ],
    [pkg({ id: "p", name: "Pulse" })],
  );

  it("searches name and description case-insensitively and combines filters", () => {
    expect(filterHubItems(items, { ...EMPTY_HUB_FILTERS, query: "SHIP" }).map((i) => i.refId)).toEqual(["1"]);
    expect(filterHubItems(items, { ...EMPTY_HUB_FILTERS, kinds: ["plugin"] }).map((i) => i.refId)).toEqual(["p"]);
    expect(filterHubItems(items, { query: "", kinds: ["skill"], sources: ["manual"] }).map((i) => i.refId)).toEqual(["2"]);
  });

  it("sorts by name and by updated with a stable tiebreak", () => {
    expect(sortHubItems(items, "name", "asc").map((i) => i.name)).toEqual(["alpha", "Deploy", "Pulse"]);
    expect(sortHubItems(items, "updated", "desc")[0]!.refId).toBe("2");
  });

  it("summarizes per source", () => {
    const summary = summarizeHubSources(items);
    expect(summary.find((s) => s.source === "manual")).toMatchObject({ count: 1, skills: 1, plugins: 0 });
    expect(summary.find((s) => s.source === "plugin")).toMatchObject({ count: 1, plugins: 1 });
  });
});
