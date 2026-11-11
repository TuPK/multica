// @vitest-environment jsdom
import { afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { EMPTY_HUB_LIST_FILTERS } from "../list";
import { setCurrentWorkspace } from "../../platform/workspace-storage";
import { useHubViewStore } from "./view-store";

const flush = () => new Promise((resolve) => queueMicrotask(() => resolve(null)));

beforeAll(() => {
  if (typeof globalThis.localStorage?.clear !== "function") {
    const values = new Map<string, string>();
    const storage: Storage = {
      get length() { return values.size; },
      clear: () => values.clear(),
      getItem: (k) => values.get(k) ?? null,
      key: (i) => Array.from(values.keys())[i] ?? null,
      removeItem: (k) => { values.delete(k); },
      setItem: (k, v) => { values.set(k, v); },
    };
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: storage });
    Object.defineProperty(window, "localStorage", { configurable: true, value: storage });
  }
});

beforeEach(() => {
  localStorage.clear();
  useHubViewStore.setState({
    view: "list",
    sortField: "updated",
    sortDirection: "desc",
    filters: EMPTY_HUB_LIST_FILTERS,
    search: "",
  });
  setCurrentWorkspace(null, null);
});

afterEach(() => {
  setCurrentWorkspace(null, null);
});

describe("useHubViewStore", () => {
  it("toggles a multi-select filter value on and off", () => {
    const { toggleFilter } = useHubViewStore.getState();
    toggleFilter("sources", "github");
    toggleFilter("sources", "clawhub");
    expect(useHubViewStore.getState().filters.sources).toEqual(["github", "clawhub"]);
    toggleFilter("sources", "github");
    expect(useHubViewStore.getState().filters.sources).toEqual(["clawhub"]);
  });

  it("replaces one dimension and clears every filter", () => {
    const { setFilterValues, toggleFilter, clearFilters } = useHubViewStore.getState();
    setFilterValues("kinds", ["plugin"]);
    toggleFilter("labels", "ops");
    expect(useHubViewStore.getState().filters).toMatchObject({ kinds: ["plugin"], labels: ["ops"] });
    clearFilters();
    expect(useHubViewStore.getState().filters).toEqual(EMPTY_HUB_LIST_FILTERS);
  });

  it("switches the sort field with its default direction and toggles the active one", () => {
    const { toggleSort } = useHubViewStore.getState();
    toggleSort("name");
    expect(useHubViewStore.getState()).toMatchObject({ sortField: "name", sortDirection: "asc" });
    toggleSort("name");
    expect(useHubViewStore.getState().sortDirection).toBe("desc");
    toggleSort("usedBy");
    expect(useHubViewStore.getState()).toMatchObject({ sortField: "usedBy", sortDirection: "desc" });
  });

  it("switches between list and catalog", () => {
    useHubViewStore.getState().setView("catalog");
    expect(useHubViewStore.getState().view).toBe("catalog");
  });

  it("persists the view and filters per workspace, but never the search text", () => {
    setCurrentWorkspace("acme", "ws_a");
    useHubViewStore.getState().setView("catalog");
    useHubViewStore.getState().toggleFilter("usage", "unused");
    useHubViewStore.getState().setSearch("deploy");

    const raw = localStorage.getItem("multica_hub_view:acme");
    expect(raw).not.toBeNull();
    const saved = JSON.parse(raw!).state;
    expect(saved).toMatchObject({ view: "catalog", filters: { usage: ["unused"] } });
    expect(saved).not.toHaveProperty("search");
  });

  it("restores the saved preferences when a workspace is entered, with the search cleared", async () => {
    localStorage.setItem(
      "multica_hub_view:acme",
      JSON.stringify({ state: { view: "catalog", sortField: "name", sortDirection: "asc", filters: { sources: ["github"] } }, version: 0 }),
    );
    useHubViewStore.getState().setSearch("left over");
    setCurrentWorkspace("acme", "ws_a");
    await flush();
    await flush();

    const state = useHubViewStore.getState();
    expect(state).toMatchObject({ view: "catalog", sortField: "name", sortDirection: "asc" });
    expect(state.filters).toEqual({ ...EMPTY_HUB_LIST_FILTERS, sources: ["github"] });
    expect(state.search).toBe("");
  });

  it("backfills filter dimensions missing from an older payload", async () => {
    localStorage.setItem(
      "multica_hub_view:acme",
      JSON.stringify({ state: { filters: { kinds: ["skill"] } }, version: 0 }),
    );
    setCurrentWorkspace("acme", "ws_a");
    await flush();
    await flush();
    expect(useHubViewStore.getState().filters).toEqual({ ...EMPTY_HUB_LIST_FILTERS, kinds: ["skill"] });
  });
});
