// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "../api/client";
import {
  HubImportBlockedError,
  HubImportWarningsError,
  HubPreviewExpiredError,
  HubSourceInUseError,
} from "./errors";

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

const SOURCE = {
  id: "src-1",
  name: "Team",
  type: "skills_sh",
  url: "https://skills.sh/acme",
  config: {},
  enabled: true,
  auto_sync_interval_sec: 0,
  last_sync_at: null,
  last_sync_status: "ok",
  last_sync_error: null,
  item_count: 2,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const PREVIEW = {
  preview_id: "pv-1",
  kind: "plugin",
  name: "tool",
  description: "",
  version: "0.1.0",
  files: [],
  scan: { status: "clean", findings: [] },
  conflict: { exists: false },
};

function requestOf(fetchMock: ReturnType<typeof vi.fn>, index = 0) {
  const [url, init] = fetchMock.mock.calls[index] as [string, RequestInit];
  return { url, init, headers: new Headers(init.headers) };
}

describe("hub source client methods", () => {
  it("lists sources with the workspace id header and tolerates a malformed body", async () => {
    const fetchMock = stubFetch(jsonResponse({ sources: "nope" }));
    const client = new ApiClient("https://api.example.test");
    await expect(client.listHubSources("ws-1")).resolves.toEqual([]);
    const { url, headers } = requestOf(fetchMock);
    expect(url).toBe("https://api.example.test/api/hub/sources");
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
    expect(headers.get("X-Workspace-Slug")).toBe("");
  });

  it("normalizes source types and fills defaults on list rows", async () => {
    stubFetch(jsonResponse([{ id: "a", type: "mystery" }, SOURCE]));
    const client = new ApiClient("https://api.example.test");
    const sources = await client.listHubSources("ws-1");
    expect(sources[0]).toMatchObject({ id: "a", type: "manual", enabled: true });
    expect(sources[1]?.type).toBe("skills_sh");
  });

  it("throws when a create response has no id instead of returning an empty source", async () => {
    stubFetch(jsonResponse({ name: "no id" }));
    const client = new ApiClient("https://api.example.test");
    await expect(
      client.createHubSource("ws-1", { name: "x", type: "github", url: "https://github.com/a/b" }),
    ).rejects.toThrow(/no source id/);
  });

  it("sends the PATCH body and encodes the source id", async () => {
    const fetchMock = stubFetch(jsonResponse(SOURCE));
    const client = new ApiClient("https://api.example.test");
    await client.updateHubSource("ws-1", "src/1", { enabled: false });
    const { url, init } = requestOf(fetchMock);
    expect(url).toBe("https://api.example.test/api/hub/sources/src%2F1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(String(init.body))).toEqual({ enabled: false });
  });

  it("resolves a 204 delete and sends the items mode", async () => {
    const fetchMock = stubFetch(new Response(null, { status: 204 }));
    const client = new ApiClient("https://api.example.test");
    await expect(client.deleteHubSource("ws-1", "src-1", "keep")).resolves.toBeUndefined();
    expect(requestOf(fetchMock).url).toBe("https://api.example.test/api/hub/sources/src-1?items=keep");
  });

  it("rejects a delete blocked by agents as HubSourceInUseError", async () => {
    stubFetch(jsonResponse({ in_use: [{ kind: "agent", id: "a1", name: "Writer" }] }, 409));
    const client = new ApiClient("https://api.example.test");
    const error = await client.deleteHubSource("ws-1", "src-1", "delete").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HubSourceInUseError);
    expect((error as HubSourceInUseError).inUse).toEqual([{ kind: "agent", id: "a1", name: "Writer" }]);
  });
});

describe("hub import client methods", () => {
  it("posts a JSON source preview", async () => {
    const fetchMock = stubFetch(jsonResponse(PREVIEW));
    const client = new ApiClient("https://api.example.test");
    const preview = await client.previewHubImport("ws-1", { type: "github", url: "https://github.com/a/b", ref: "v1" });
    expect(preview.preview_id).toBe("pv-1");
    const { url, init, headers } = requestOf(fetchMock);
    expect(url).toBe("https://api.example.test/api/hub/import/preview");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(JSON.parse(String(init.body))).toEqual({ source: { type: "github", url: "https://github.com/a/b", ref: "v1" } });
  });

  it("uploads a zip as multipart without a JSON content type", async () => {
    const fetchMock = stubFetch(jsonResponse(PREVIEW));
    const client = new ApiClient("https://api.example.test");
    const file = new File([new Uint8Array([80, 75])], "tool.zip", { type: "application/zip" });
    await client.previewHubImportUpload("ws-1", file);
    const { init, headers } = requestOf(fetchMock);
    expect(init.method).toBe("POST");
    expect(init.body).toBeInstanceOf(FormData);
    const sent = (init.body as FormData).get("file");
    expect(sent).toBeInstanceOf(File);
    expect((sent as File).name).toBe("tool.zip");
    expect(headers.get("Content-Type")).toBeNull();
    expect(headers.get("X-Workspace-ID")).toBe("ws-1");
  });

  it("rejects a preview body with no preview id", async () => {
    stubFetch(jsonResponse({ kind: "skill" }));
    const client = new ApiClient("https://api.example.test");
    await expect(client.previewHubImport("ws-1", { type: "url_zip", url: "https://x/y.zip" })).rejects.toThrow(
      /no preview id/,
    );
  });

  it("maps a blocked commit to HubImportBlockedError", async () => {
    stubFetch(jsonResponse({ error: "scan blocked", code: "scan_blocked" }, 409));
    const client = new ApiClient("https://api.example.test");
    await expect(
      client.commitHubImport("ws-1", { preview_id: "pv-1", on_conflict: "rename", confirm_warnings: false }),
    ).rejects.toBeInstanceOf(HubImportBlockedError);
  });

  it("maps an unconfirmed-warning commit to HubImportWarningsError", async () => {
    stubFetch(jsonResponse({ error: "confirm", code: "warnings_unconfirmed" }, 409));
    const client = new ApiClient("https://api.example.test");
    await expect(
      client.commitHubImport("ws-1", { preview_id: "pv-1", on_conflict: "skip", confirm_warnings: false }),
    ).rejects.toBeInstanceOf(HubImportWarningsError);
  });

  it("maps an expired preview to HubPreviewExpiredError", async () => {
    stubFetch(jsonResponse({ error: "expired" }, 410));
    const client = new ApiClient("https://api.example.test");
    await expect(
      client.commitHubImport("ws-1", { preview_id: "pv-1", on_conflict: "replace", confirm_warnings: true }),
    ).rejects.toBeInstanceOf(HubPreviewExpiredError);
  });

  it("keeps an unrecognised 409 as a plain ApiError", async () => {
    stubFetch(jsonResponse({ error: "busy" }, 409));
    const client = new ApiClient("https://api.example.test");
    const error = await client
      .commitHubImport("ws-1", { preview_id: "pv-1", on_conflict: "rename", confirm_warnings: false })
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
  });

  it("sends the commit body with snake_case keys and returns the parsed result", async () => {
    const fetchMock = stubFetch(jsonResponse({ kind: "skill", id: "sk-1", hub_item_id: "hi-1", skipped: false }));
    const client = new ApiClient("https://api.example.test");
    const result = await client.commitHubImport("ws-1", {
      preview_id: "pv-1",
      on_conflict: "rename",
      name: "Renamed",
      confirm_warnings: true,
      source_id: "src-1",
    });
    expect(result).toEqual({ kind: "skill", id: "sk-1", hub_item_id: "hi-1", skipped: false });
    expect(JSON.parse(String(requestOf(fetchMock).init.body))).toEqual({
      preview_id: "pv-1",
      on_conflict: "rename",
      name: "Renamed",
      confirm_warnings: true,
      source_id: "src-1",
    });
  });

  it("refuses a commit response that carries no item id", async () => {
    stubFetch(jsonResponse({ kind: "skill", skipped: false }));
    const client = new ApiClient("https://api.example.test");
    await expect(
      client.commitHubImport("ws-1", { preview_id: "pv-1", on_conflict: "rename", confirm_warnings: false }),
    ).rejects.toThrow(/without an item id/);
  });
});
