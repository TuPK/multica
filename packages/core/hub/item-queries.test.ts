// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { createQueryClient } from "../query-client";
import type { HubItemMeta } from "../types/hub";
import { hubActivityInfiniteOptions, HUB_ACTIVITY_PAGE_SIZE } from "./activity";
import { hubItemKeys } from "./item-keys";
import { findHubItemMeta, hubItemMetaListOptions } from "./item-meta";
import { downloadHubBundle, fetchHubBundleDownload } from "./bundle";
import { hubQuotaOptions } from "./quota";
import { hubUpdateCheckOptions, hubUpdateDiffOptions } from "./update";
import { hubKeys } from "./files";

afterEach(() => {
  setApiInstance(null as unknown as ApiClient);
});

function meta(overrides: Partial<HubItemMeta>): HubItemMeta {
  return {
    id: "hi-x",
    kind: "skill",
    ref_id: "sk-1",
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
    ...overrides,
  };
}

describe("hub item query keys", () => {
  it.each([
    ["items", () => hubItemKeys.items("ws-1")],
    ["updates", () => hubItemKeys.updates("ws-1")],
    ["update", () => hubItemKeys.update("ws-1", "skill", "sk-1")],
    ["updateDiff", () => hubItemKeys.updateDiff("ws-1", "plugin", "p1", "a.js")],
    ["activity", () => hubItemKeys.activity("ws-1", { kind: "skill" })],
    ["activityAll", () => hubItemKeys.activityAll("ws-1")],
    ["quota", () => hubItemKeys.quota("ws-1")],
  ])("scopes %s by workspace", (_name, key) => {
    expect(key()).toContain("ws-1");
    expect(key()[0]).toBe("workspaces");
    expect(key()).not.toContain("ws-2");
  });

  it("keeps the same workspace's keys under the hub prefix so one invalidation covers them", () => {
    expect(hubItemKeys.items("ws-1").slice(0, 3)).toEqual(hubKeys.all("ws-1"));
    expect(hubItemKeys.update("ws-1", "skill", "s").slice(0, 3)).toEqual(hubKeys.all("ws-1"));
  });

  it("separates update checks by item and by file path", () => {
    expect(hubItemKeys.update("ws-1", "skill", "a")).not.toEqual(hubItemKeys.update("ws-1", "skill", "b"));
    expect(hubItemKeys.updateDiff("ws-1", "skill", "a", "x.md")).not.toEqual(
      hubItemKeys.updateDiff("ws-1", "skill", "a", "y.md"),
    );
  });

  it("treats an omitted activity filter the same as an empty one", () => {
    expect(hubItemKeys.activity("ws-1")).toEqual(hubItemKeys.activity("ws-1", {}));
    expect(hubItemKeys.activity("ws-1", { refId: "sk-1" })).not.toEqual(hubItemKeys.activity("ws-1"));
  });
});

describe("item metadata lookup", () => {
  const items = [
    meta({ id: "hi-1", kind: "skill", ref_id: "sk-1" }),
    meta({ id: "hi-2", kind: "plugin", ref_id: "sk-1" }),
  ];

  it("matches on kind and ref id together", () => {
    expect(findHubItemMeta(items, "plugin", "sk-1")?.id).toBe("hi-2");
    expect(findHubItemMeta(items, "skill", "sk-1")?.id).toBe("hi-1");
  });

  it("returns undefined when no row matches", () => {
    expect(findHubItemMeta(items, "skill", "missing")).toBeUndefined();
  });

  it("lists items under the workspace key and only when a workspace is set", () => {
    expect(hubItemMetaListOptions("ws-1").queryKey).toEqual(hubItemKeys.items("ws-1"));
    expect(hubItemMetaListOptions("").enabled).toBe(false);
  });
});

describe("update and quota query options", () => {
  it("disables update checks until the item is named", () => {
    expect(hubUpdateCheckOptions("ws-1", "skill", "").enabled).toBe(false);
    expect(hubUpdateCheckOptions("ws-1", "skill", "sk-1").enabled).toBe(true);
  });

  it("disables diffs until a path is chosen", () => {
    expect(hubUpdateDiffOptions("ws-1", "skill", "sk-1", "").enabled).toBe(false);
    expect(hubUpdateDiffOptions("ws-1", "skill", "sk-1", "a.md").enabled).toBe(true);
  });

  it("keys quota by workspace and disables it without one", () => {
    expect(hubQuotaOptions("ws-1").queryKey).toEqual(hubItemKeys.quota("ws-1"));
    expect(hubQuotaOptions("").enabled).toBe(false);
  });
});

describe("bundle download url", () => {
  it("returns an absolute presigned url unchanged", async () => {
    setApiInstance({
      getHubBundle: vi.fn().mockResolvedValue({ url: "https://store.example/x?sig=1", expires_in: 60, digest: "", size_bytes: 0, version: "" }),
      getBaseUrl: () => "https://api.example.test",
    } as unknown as ApiClient);
    await expect(downloadHubBundle("ws-1", "skill", "sk-1")).resolves.toBe("https://store.example/x?sig=1");
  });

  it("makes a same-origin download path absolute against the API base", async () => {
    setApiInstance({
      getHubBundle: vi.fn().mockResolvedValue({ url: "/api/hub/items/skill/sk-1/bundle/download", expires_in: 0, digest: "", size_bytes: 0, version: "" }),
      getBaseUrl: () => "https://api.example.test",
    } as unknown as ApiClient);
    await expect(downloadHubBundle("ws-1", "skill", "sk-1")).resolves.toBe(
      "https://api.example.test/api/hub/items/skill/sk-1/bundle/download",
    );
  });
});

describe("fetchHubBundleDownload", () => {
  it("returns an absolute presigned url for the view to open", async () => {
    setApiInstance({
      getHubBundle: vi.fn().mockResolvedValue({ url: "https://store.example/x?sig=1", expires_in: 60, digest: "d", size_bytes: 1, version: "2" }),
    } as unknown as ApiClient);
    await expect(fetchHubBundleDownload("ws-1", "skill", "sk-1")).resolves.toEqual({
      kind: "url",
      url: "https://store.example/x?sig=1",
      version: "2",
    });
  });

  it("fetches a relative download route with auth and returns the bytes", async () => {
    const blob = new Blob(["zip"]);
    const getHubBundleBlob = vi.fn().mockResolvedValue(blob);
    setApiInstance({
      getHubBundle: vi.fn().mockResolvedValue({ url: "/api/hub/items/skill/sk-1/bundle/download", expires_in: 0, digest: "d", size_bytes: 3, version: "2" }),
      getHubBundleBlob,
    } as unknown as ApiClient);
    await expect(fetchHubBundleDownload("ws-1", "skill", "sk-1")).resolves.toEqual({
      kind: "blob",
      blob,
      version: "2",
      digest: "d",
    });
    expect(getHubBundleBlob).toHaveBeenCalledWith("ws-1", "/api/hub/items/skill/sk-1/bundle/download");
  });
});

describe("activity cursor paging", () => {
  it("requests the first page with no cursor and follows next_cursor until it is null", async () => {
    const getHubActivityPage = vi.fn(async (_wsId: string, params: { before?: string | null; limit?: number; kind?: string }) => {
      if (params.before == null) return { entries: [{ id: "e1" }], next_cursor: "c-1" };
      if (params.before === "c-1") return { entries: [{ id: "e2" }], next_cursor: "c-2" };
      return { entries: [{ id: "e3" }], next_cursor: null };
    });
    setApiInstance({ listHubActivity: getHubActivityPage } as unknown as ApiClient);

    const qc = createQueryClient();
    const data = await qc.fetchInfiniteQuery({ ...hubActivityInfiniteOptions("ws-1", { kind: "skill" }), pages: 5 });

    expect(data.pages.map((page) => page.entries.map((entry) => entry.id))).toEqual([["e1"], ["e2"], ["e3"]]);
    expect(getHubActivityPage.mock.calls.map((call) => call[1].before)).toEqual([null, "c-1", "c-2"]);
    expect(getHubActivityPage.mock.calls.every((call) => call[1].limit === HUB_ACTIVITY_PAGE_SIZE)).toBe(true);
    expect(getHubActivityPage.mock.calls.every((call) => call[1].kind === "skill")).toBe(true);
    qc.clear();
  });

  it("stops at the first page when the server reports no cursor", async () => {
    const listHubActivity = vi.fn().mockResolvedValue({ entries: [], next_cursor: null });
    setApiInstance({ listHubActivity } as unknown as ApiClient);
    const qc = createQueryClient();
    const options = hubActivityInfiniteOptions("ws-1");
    expect(options.getNextPageParam({ entries: [], next_cursor: null }, [], null, [])).toBeUndefined();
    await qc.fetchInfiniteQuery({ ...options, pages: 3 });
    expect(listHubActivity).toHaveBeenCalledTimes(1);
    qc.clear();
  });

  it("keys each filter separately so one item's activity never fills another's pages", () => {
    expect(hubActivityInfiniteOptions("ws-1", { refId: "a" }).queryKey).not.toEqual(
      hubActivityInfiniteOptions("ws-1", { refId: "b" }).queryKey,
    );
  });
});
