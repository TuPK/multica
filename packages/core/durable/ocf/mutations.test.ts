// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api";
import { buildOCFExportPath, downloadOCFExport } from "./mutations";

const { getBaseUrl, getToken } = vi.hoisted(() => ({
  getBaseUrl: vi.fn(() => "https://api.example.com/"),
  getToken: vi.fn(() => "tok-123"),
}));

vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { getBaseUrl, getToken },
}));

vi.mock("../../platform", () => ({
  getCurrentSlug: () => "acme",
}));

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("buildOCFExportPath", () => {
  it("omits include_principal when history is false", () => {
    expect(buildOCFExportPath({ history: false })).toBe(
      "/api/company/export?format=ocf&history=false",
    );
    expect(buildOCFExportPath({ history: false, include_principal: true })).toBe(
      "/api/company/export?format=ocf&history=false",
    );
  });

  it("sets history=true and includes include_principal only when true", () => {
    expect(buildOCFExportPath({ history: true })).toBe(
      "/api/company/export?format=ocf&history=true",
    );
    expect(buildOCFExportPath({ history: true, include_principal: false })).toBe(
      "/api/company/export?format=ocf&history=true",
    );
    expect(buildOCFExportPath({ history: true, include_principal: true })).toBe(
      "/api/company/export?format=ocf&history=true&include_principal=true",
    );
  });
});

describe("downloadOCFExport", () => {
  it("fetches the OCF zip with auth and workspace headers", async () => {
    const zipBlob = new Blob(["zip-bytes"], { type: "application/zip" });
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      blob: () => Promise.resolve(zipBlob),
    });
    vi.stubGlobal("fetch", fetchMock);

    const out = await downloadOCFExport("ws-1", { history: true, include_principal: true });

    expect(out).toBe(zipBlob);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/api/company/export?format=ocf&history=true&include_principal=true",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
        headers: {
          Authorization: "Bearer tok-123",
          "X-Workspace-Slug": "acme",
          "X-Workspace-ID": "ws-1",
        },
      }),
    );
  });

  it("throws ApiError when the server returns an error status", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
      json: () => Promise.resolve({ error: "failed to export the company", code: "export_failed" }),
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(downloadOCFExport("ws-1", { history: false })).rejects.toMatchObject({
      name: "ApiError",
      status: 500,
      message: "failed to export the company",
      body: { error: "failed to export the company", code: "export_failed" },
    } satisfies Partial<ApiError>);
  });

  it.each([
    ["non-object JSON", () => Promise.resolve(null)],
    ["missing error", () => Promise.resolve({})],
    ["invalid error", () => Promise.resolve({ error: 42 })],
    ["unreadable JSON", () => Promise.reject(new Error("invalid JSON"))],
  ])("falls back for %s", async (_label, json) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: false, status: 502, statusText: "Bad Gateway", json,
    }));
    await expect(downloadOCFExport("ws-1", { history: false })).rejects.toMatchObject({
      name: "ApiError", status: 502, message: "API error: 502 Bad Gateway",
    });
  });

  it("validates the error response before passing it to ApiError", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: false, status: 400, statusText: "Bad Request", json: () => Promise.resolve(["bad"]),
    }));
    const error = await downloadOCFExport("ws-1", { history: false }).catch((err: ApiError) => err);
    expect(error.body).toEqual({});
  });
});
