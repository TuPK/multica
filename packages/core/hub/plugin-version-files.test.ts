// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "../api/client";
import {
  EMPTY_PLUGIN_VERSION_FILE_LIST,
  PluginVersionFileContentSchema,
  PluginVersionFileListResponseSchema,
} from "../api/schemas";
import { parseWithFallback } from "../api/schema";

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse(body));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("ApiClient plugin version files", () => {
  it("lists the files of one version", async () => {
    const fetchMock = stubFetch({ files: [{ path: "ui/panel.html", size_bytes: 12, sha256: "abc" }] });
    const client = new ApiClient("https://api.example.test");
    const result = await client.listPluginVersionFiles("ws-1", "pkg-1", "v-1");
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "https://api.example.test/api/workspaces/ws-1/plugins/packages/pkg-1/versions/v-1/files",
    );
    expect(result.files).toEqual([{ path: "ui/panel.html", size_bytes: 12, sha256: "abc" }]);
  });

  it("encodes the path in the content query", async () => {
    const fetchMock = stubFetch({ path: "a b/&c.js", content: "x", is_binary: false, size_bytes: 1, sha256: "h" });
    const client = new ApiClient("https://api.example.test");
    await client.getPluginVersionFileContent("ws-1", "pkg-1", "v-1", "a b/&c.js");
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "https://api.example.test/api/workspaces/ws-1/plugins/packages/pkg-1/versions/v-1/files/content?path=a%20b%2F%26c.js",
    );
  });

  it("returns binary content with null content", async () => {
    stubFetch({ path: "icon.png", content: null, is_binary: true, size_bytes: 900, sha256: "h" });
    const client = new ApiClient("https://api.example.test");
    const result = await client.getPluginVersionFileContent("ws-1", "pkg-1", "v-1", "icon.png");
    expect(result).toEqual({ path: "icon.png", content: null, is_binary: true, size_bytes: 900, sha256: "h" });
  });

  it("falls back to an empty list on a malformed listing", async () => {
    stubFetch({ files: "not-a-list" });
    const client = new ApiClient("https://api.example.test");
    expect((await client.listPluginVersionFiles("ws-1", "pkg-1", "v-1")).files).toEqual([]);
  });

  it("reports a malformed content response as unreadable, never as text", async () => {
    stubFetch({ path: "a.js", content: 42, is_binary: false, size_bytes: 2, sha256: "h" });
    const client = new ApiClient("https://api.example.test");
    const result = await client.getPluginVersionFileContent("ws-1", "pkg-1", "v-1", "a.js");
    expect(result).toEqual({ path: "a.js", content: null, is_binary: true, size_bytes: 0, sha256: "" });
  });
});

describe("ApiClient skill file writes", () => {
  it("PUTs a skill file and returns its updated_at", async () => {
    const fetchMock = stubFetch({
      id: "f-1", skill_id: "s-1", path: "notes.md", content: "hi", created_at: "c", updated_at: "2026-02-01T00:00:00Z",
    });
    const client = new ApiClient("https://api.example.test");
    const saved = await client.upsertSkillFile("s-1", { path: "notes.md", content: "hi" });
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("PUT");
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({ path: "notes.md", content: "hi" });
    expect(saved.updated_at).toBe("2026-02-01T00:00:00Z");
  });

  it("falls back to the requested path with an empty etag on a malformed upsert response", async () => {
    stubFetch({ id: 7, path: "notes.md" });
    const client = new ApiClient("https://api.example.test");
    const saved = await client.upsertSkillFile("s-1", { path: "notes.md", content: "hi" });
    expect(saved).toMatchObject({ path: "notes.md", content: "hi", updated_at: "" });
  });

  it("DELETEs a skill file by id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new ApiClient("https://api.example.test");
    await client.deleteSkillFile("s-1", "f-1");
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("https://api.example.test/api/skills/s-1/files/f-1");
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("DELETE");
  });
});

describe("plugin version file schemas", () => {
  it("defaults missing optional fields on a content response", () => {
    const parsed = parseWithFallback(
      { path: "a.js" },
      PluginVersionFileContentSchema,
      { path: "fallback", content: null, is_binary: true, size_bytes: 0, sha256: "" },
      { endpoint: "test" },
    );
    expect(parsed).toEqual({ path: "a.js", content: null, is_binary: false, size_bytes: 0, sha256: "" });
  });

  it("rejects a listing entry without a path, so the whole listing falls back", () => {
    const parsed = parseWithFallback(
      { files: [{ size_bytes: 1, sha256: "h" }] },
      PluginVersionFileListResponseSchema,
      EMPTY_PLUGIN_VERSION_FILE_LIST,
      { endpoint: "test" },
    );
    expect(parsed).toEqual({ files: [] });
  });

  it("accepts unknown extra fields from a newer backend", () => {
    const parsed = parseWithFallback(
      { files: [{ path: "a.js", size_bytes: 1, sha256: "h", mode: "0644" }] },
      PluginVersionFileListResponseSchema,
      EMPTY_PLUGIN_VERSION_FILE_LIST,
      { endpoint: "test" },
    );
    expect(parsed.files).toHaveLength(1);
    expect(parsed.files[0]?.path).toBe("a.js");
  });
});
