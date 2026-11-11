// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "../api/client";
import { HubFileConflictError, HubLocalChangesError, HubQuotaExceededError } from "./errors";

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubFetch(...responses: Response[]) {
  const fetchMock = vi.fn();
  for (const response of responses) fetchMock.mockResolvedValueOnce(response);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function requestOf(fetchMock: ReturnType<typeof vi.fn>, index = 0) {
  const [url, init] = fetchMock.mock.calls[index] as [string, RequestInit];
  return { url, init, headers: new Headers(init.headers) };
}

const BASE = "https://api.example.test";

const ITEM = {
  id: "hi-1",
  kind: "skill",
  ref_id: "sk-1",
  source_id: "src-1",
  source_ref: "acme/writer",
  tracking: "forked",
  latest_version: "1.2.0",
  bundle_digest: "sha256:aaa",
  size_bytes: 2048,
  file_count: 3,
  scan_status: "clean",
  update_available: true,
  remote_digest: "sha256:bbb",
  checked_at: null,
  local_modified: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-03-01T00:00:00Z",
};

describe("hub item list and bundle client methods", () => {
  it("lists items with the workspace header and parses every row", async () => {
    const fetchMock = stubFetch(jsonResponse([ITEM, { id: "hi-2", tracking: "mystery" }]));
    const client = new ApiClient(BASE);
    const items = await client.listHubItems("ws-1");
    const { url, headers } = requestOf(fetchMock);
    expect(url).toBe(`${BASE}/api/hub/items`);
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(items[0]).toMatchObject({ ref_id: "sk-1", tracking: "forked", local_modified: true });
    expect(items[1]).toMatchObject({ id: "hi-2", tracking: "manual", kind: "skill" });
  });

  it("returns an empty list for a malformed list body", async () => {
    stubFetch(jsonResponse({ items: [ITEM] }));
    const client = new ApiClient(BASE);
    await expect(client.listHubItems("ws-1")).resolves.toEqual([]);
  });

  it("requests a bundle for the ref id and returns the url and digest", async () => {
    const fetchMock = stubFetch(
      jsonResponse({ url: "https://store/x?sig=1", expires_in: 900, digest: "sha256:aaa", size_bytes: 2048, version: "1.2.0" }),
    );
    const client = new ApiClient(BASE);
    const bundle = await client.getHubBundle("ws-1", "plugin", "pkg/1");
    expect(requestOf(fetchMock).url).toBe(`${BASE}/api/hub/items/plugin/pkg%2F1/bundle`);
    expect(bundle).toMatchObject({ url: "https://store/x?sig=1", expires_in: 900, version: "1.2.0" });
  });

  it("refuses a bundle response with no url instead of returning an unusable link", async () => {
    stubFetch(jsonResponse({ expires_in: 900 }));
    const client = new ApiClient(BASE);
    await expect(client.getHubBundle("ws-1", "skill", "sk-1")).rejects.toThrow(/no download url/);
  });
});

describe("hub sync and update client methods", () => {
  it("posts a sync for the source and parses per-item changes", async () => {
    const fetchMock = stubFetch(
      jsonResponse({
        status: "ok",
        checked: 1,
        updates_available: 1,
        unchanged: 0,
        items: [{ kind: "skill", id: "sk-1", name: "Writer", change: "update_available" }],
      }),
    );
    const client = new ApiClient(BASE);
    const result = await client.syncHubSource("ws-1", "src 1");
    const { url, init, headers } = requestOf(fetchMock);
    expect(url).toBe(`${BASE}/api/hub/sources/src%201/sync`);
    expect(init.method).toBe("POST");
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(result.items).toEqual([{ kind: "skill", id: "sk-1", name: "Writer", change: "update_available" }]);
  });

  it("reports a malformed sync body as an error status rather than a clean run", async () => {
    stubFetch(jsonResponse("not an object"));
    const client = new ApiClient(BASE);
    await expect(client.syncHubSource("ws-1", "src-1")).resolves.toMatchObject({
      status: "error",
      checked: 0,
      items: [],
    });
  });

  it("reads the update check and falls back for a malformed file list", async () => {
    const fetchMock = stubFetch(
      jsonResponse({ update_available: true, current_digest: "a", remote_digest: "b", local_modified: false, files: "x" }),
    );
    const client = new ApiClient(BASE);
    const check = await client.getHubUpdateCheck("ws-1", "skill", "sk-1");
    expect(requestOf(fetchMock).url).toBe(`${BASE}/api/hub/items/skill/sk-1/update`);
    expect(check).toMatchObject({ update_available: true, remote_digest: "b", files: [] });
  });

  it("encodes the diff path in the query string", async () => {
    const fetchMock = stubFetch(jsonResponse({ path: "docs/a b.md", old: "x", new: "y", binary: false }));
    const client = new ApiClient(BASE);
    const diff = await client.getHubUpdateDiff("ws-1", "skill", "sk-1", "docs/a b.md");
    expect(requestOf(fetchMock).url).toBe(`${BASE}/api/hub/items/skill/sk-1/update/diff?path=docs%2Fa%20b.md`);
    expect(diff).toEqual({ path: "docs/a b.md", old: "x", new: "y", binary: false });
  });

  it("sends the chosen local-changes policy when applying an update", async () => {
    const fetchMock = stubFetch(jsonResponse({ kind: "skill", id: "sk-1", version: "1.2.0", tracking: "tracking" }));
    const client = new ApiClient(BASE);
    const applied = await client.applyHubUpdate("ws-1", "skill", "sk-1", "fork");
    const { url, init, headers } = requestOf(fetchMock);
    expect(url).toBe(`${BASE}/api/hub/items/skill/sk-1/update/apply`);
    expect(init.method).toBe("POST");
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(JSON.parse(String(init.body))).toEqual({ on_local_changes: "fork" });
    expect(applied).toEqual({ kind: "skill", id: "sk-1", version: "1.2.0", tracking: "tracking" });
  });

  it("maps a local_changes 409 to HubLocalChangesError", async () => {
    stubFetch(jsonResponse({ error: "edited locally", code: "local_changes" }, 409));
    const client = new ApiClient(BASE);
    await expect(client.applyHubUpdate("ws-1", "skill", "sk-1", "overwrite")).rejects.toBeInstanceOf(
      HubLocalChangesError,
    );
  });

  it("maps a quota_exceeded 409 on apply to HubQuotaExceededError", async () => {
    stubFetch(jsonResponse({ error: "full", code: "quota_exceeded" }, 409));
    const client = new ApiClient(BASE);
    await expect(client.applyHubUpdate("ws-1", "plugin", "pkg-1", "overwrite")).rejects.toBeInstanceOf(
      HubQuotaExceededError,
    );
  });

  it("keeps unrecognised 409 codes on apply as plain ApiError", async () => {
    stubFetch(jsonResponse({ error: "busy", code: "something_else" }, 409));
    const client = new ApiClient(BASE);
    const error = await client.applyHubUpdate("ws-1", "skill", "sk-1", "overwrite").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
  });

  it("refuses an apply response with no item id", async () => {
    stubFetch(jsonResponse({ kind: "skill", version: "1.2.0" }));
    const client = new ApiClient(BASE);
    await expect(client.applyHubUpdate("ws-1", "skill", "sk-1", "overwrite")).rejects.toThrow(/no item id/);
  });

  it("forks an item and returns the forked tracking mode", async () => {
    const fetchMock = stubFetch(jsonResponse({ kind: "skill", id: "sk-1", tracking: "forked" }));
    const client = new ApiClient(BASE);
    const forked = await client.forkHubItem("ws-1", "skill", "sk-1");
    expect(requestOf(fetchMock).init.method).toBe("POST");
    expect(requestOf(fetchMock).url).toBe(`${BASE}/api/hub/items/skill/sk-1/fork`);
    expect(forked).toEqual({ kind: "skill", id: "sk-1", tracking: "forked" });
  });
});

describe("hub activity, quota and export client methods", () => {
  it("sends the filter, limit and cursor as query parameters", async () => {
    const fetchMock = stubFetch(jsonResponse({ entries: [], next_cursor: null }));
    const client = new ApiClient(BASE);
    await client.listHubActivity("ws-1", { kind: "skill", refId: "sk-1", sourceId: "src-1", limit: 50, before: "c-9" });
    const url = new URL(requestOf(fetchMock).url);
    expect(url.pathname).toBe("/api/hub/activity");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      kind: "skill",
      ref_id: "sk-1",
      source_id: "src-1",
      limit: "50",
      before: "c-9",
    });
  });

  it("omits the cursor on the first page", async () => {
    const fetchMock = stubFetch(jsonResponse({ entries: [], next_cursor: null }));
    const client = new ApiClient(BASE);
    await client.listHubActivity("ws-1", { limit: 50, before: null });
    expect(requestOf(fetchMock).url).toBe(`${BASE}/api/hub/activity?limit=50`);
  });

  it("returns the entries and next cursor, and an empty page for a malformed body", async () => {
    stubFetch(
      jsonResponse({
        entries: [{ id: "e1", action: "item_forked", actor_name: "Ada", kind: "skill", ref_id: "sk-1", detail: {} }],
        next_cursor: "c-1",
      }),
      jsonResponse({ entries: "nope" }),
    );
    const client = new ApiClient(BASE);
    const page = await client.listHubActivity("ws-1");
    expect(page.next_cursor).toBe("c-1");
    expect(page.entries[0]).toMatchObject({ action: "item_forked", actor_name: "Ada" });
    await expect(client.listHubActivity("ws-1")).resolves.toEqual({ entries: [], next_cursor: null });
  });

  it("reads the quota with unlimited limits kept as returned", async () => {
    const fetchMock = stubFetch(jsonResponse({ items: { used: 4, limit: 0 }, storage_bytes: { used: 9, limit: -1 } }));
    const client = new ApiClient(BASE);
    await expect(client.getHubQuota("ws-1")).resolves.toEqual({
      items: { used: 4, limit: 0 },
      storage_bytes: { used: 9, limit: -1 },
    });
    expect(requestOf(fetchMock).url).toBe(`${BASE}/api/hub/quota`);
  });

  it("returns the export zip as a blob with the workspace header", async () => {
    const fetchMock = stubFetch(
      new Response(new Uint8Array([80, 75, 3, 4]), { status: 200, headers: { "Content-Type": "application/zip" } }),
    );
    const client = new ApiClient(BASE);
    const blob = await client.getHubExportBlob("ws-1");
    const { url, headers } = requestOf(fetchMock);
    expect(url).toBe(`${BASE}/api/hub/export`);
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(blob.size).toBe(4);
  });

  it("maps a quota_exceeded 409 on import commit to HubQuotaExceededError", async () => {
    stubFetch(jsonResponse({ error: "full", code: "quota_exceeded" }, 409));
    const client = new ApiClient(BASE);
    await expect(
      client.commitHubImport("ws-1", { preview_id: "pv-1", on_conflict: "rename", confirm_warnings: false }),
    ).rejects.toBeInstanceOf(HubQuotaExceededError);
  });
});

describe("hub item detail, skill file save and quota-blocked sync", () => {
  it("reads one item from the detail route and parses it like a list row", async () => {
    const fetchMock = stubFetch(jsonResponse(ITEM));
    const client = new ApiClient(BASE);
    const item = await client.getHubItem("ws-1", "skill", "sk-1");
    const { url, headers } = requestOf(fetchMock);
    expect(url).toBe(`${BASE}/api/hub/items/skill/sk-1`);
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(item).toMatchObject({ id: "hi-1", ref_id: "sk-1", tracking: "forked", update_available: true });
  });

  it("refuses an item body without an id", async () => {
    stubFetch(jsonResponse({ ref_id: "sk-1" }));
    const client = new ApiClient(BASE);
    await expect(client.getHubItem("ws-1", "skill", "sk-1")).rejects.toThrow(/no id/);
  });

  it("puts the file with its etag to the item route and returns the new etag", async () => {
    const fetchMock = stubFetch(jsonResponse({ path: "docs/a.md", etag: "2026-04-01T00:00:00Z" }));
    const client = new ApiClient(BASE);
    const saved = await client.saveHubSkillFile("ws-1", "sk 1", {
      path: "docs/a.md",
      content: "hello",
      etag: "2026-03-01T00:00:00Z",
    });
    const { url, init, headers } = requestOf(fetchMock);
    expect(url).toBe(`${BASE}/api/hub/items/skill/sk%201/files`);
    expect(init.method).toBe("PUT");
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(JSON.parse(String(init.body))).toEqual({ path: "docs/a.md", content: "hello", etag: "2026-03-01T00:00:00Z" });
    expect(saved).toEqual({ path: "docs/a.md", etag: "2026-04-01T00:00:00Z" });
  });

  it("maps a 409 file_conflict to HubFileConflictError carrying the server's current etag", async () => {
    stubFetch(
      jsonResponse({ error: "file changed", code: "file_conflict", etag: "2026-05-05T00:00:00Z" }, 409),
    );
    const client = new ApiClient(BASE);
    const error = await client
      .saveHubSkillFile("ws-1", "sk-1", { path: "a.md", content: "x", etag: "old" })
      .catch((err: unknown) => err);
    expect(error).toBeInstanceOf(HubFileConflictError);
    expect((error as HubFileConflictError).currentEtag).toBe("2026-05-05T00:00:00Z");
  });

  it("passes other save errors through unchanged", async () => {
    stubFetch(jsonResponse({ error: "boom" }, 500));
    const client = new ApiClient(BASE);
    const error = await client
      .saveHubSkillFile("ws-1", "sk-1", { path: "a.md", content: "x", etag: "" })
      .catch((err: unknown) => err);
    expect(error).toBeInstanceOf(ApiError);
  });

  it("rejects a save response with no etag rather than returning an unusable one", async () => {
    stubFetch(jsonResponse({ path: "a.md" }));
    const client = new ApiClient(BASE);
    await expect(
      client.saveHubSkillFile("ws-1", "sk-1", { path: "a.md", content: "x", etag: "" }),
    ).rejects.toThrow(/no etag/);
  });

  it("maps a sync blocked by the quota to HubQuotaExceededError", async () => {
    stubFetch(jsonResponse({ error: "limit reached", code: "quota_exceeded" }, 409));
    const client = new ApiClient(BASE);
    await expect(client.syncHubSource("ws-1", "src-1")).rejects.toBeInstanceOf(HubQuotaExceededError);
  });

  it("parses has_credential and never expects the token back", async () => {
    stubFetch(jsonResponse([{ id: "src-1", name: "Acme", type: "github", url: "https://github.com/a/b", has_credential: true }]));
    const client = new ApiClient(BASE);
    const [source] = await client.listHubSources("ws-1");
    expect(source?.has_credential).toBe(true);
  });

  it("sends a write-only credential when creating a source", async () => {
    const fetchMock = stubFetch(jsonResponse({ id: "src-1", name: "Acme", type: "github", url: "https://github.com/a/b", has_credential: true }));
    const client = new ApiClient(BASE);
    const source = await client.createHubSource("ws-1", {
      name: "Acme",
      type: "github",
      url: "https://github.com/a/b",
      credential: "ghp_secret",
    });
    const { init } = requestOf(fetchMock);
    expect(JSON.parse(String(init.body))).toMatchObject({ credential: "ghp_secret" });
    expect(source.has_credential).toBe(true);
  });

  it("sends clear_credential to remove a stored token", async () => {
    const fetchMock = stubFetch(jsonResponse({ id: "src-1", name: "Acme", type: "github", url: "https://github.com/a/b", has_credential: false }));
    const client = new ApiClient(BASE);
    const source = await client.updateHubSource("ws-1", "src-1", { clear_credential: true });
    const { init } = requestOf(fetchMock);
    expect(JSON.parse(String(init.body))).toEqual({ clear_credential: true });
    expect(source.has_credential).toBe(false);
  });
});
