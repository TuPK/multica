import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "../api/client";
import { auditQueryString } from "./params";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(res: Response) {
  const fetchMock = vi.fn().mockResolvedValue(res);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("auditQueryString", () => {
  it("is empty without parameters and skips unset or empty ones", () => {
    expect(auditQueryString()).toBe("");
    expect(auditQueryString({ action: "", from: undefined })).toBe("");
  });

  it("maps filters, paging and format to the server's names", () => {
    const qs = auditQueryString(
      { action: "pat.created", actorId: "u1", from: "2026-01-01T00:00:00Z", to: "2026-02-01T00:00:00Z" },
      { limit: 50, cursor: "c1" },
      "csv",
    );
    const p = new URLSearchParams(qs);
    expect(Object.fromEntries(p)).toEqual({
      action: "pat.created",
      actor_id: "u1",
      from: "2026-01-01T00:00:00Z",
      to: "2026-02-01T00:00:00Z",
      limit: "50",
      cursor: "c1",
      format: "csv",
    });
  });
});

describe("ApiClient.exportWorkspaceAuditCsv", () => {
  it("requests format=csv with the filters and returns the text", async () => {
    const fetchMock = stubFetch(
      new Response("id,action\ne1,auth.login\n", { status: 200, headers: { "Content-Type": "text/csv" } }),
    );

    const result = await new ApiClient("https://api.test").exportWorkspaceAuditCsv("w1", {
      action: "auth.login",
    });

    expect(result).toEqual({ text: "id,action\ne1,auth.login\n", truncated: false });
    const url = new URL(fetchMock.mock.calls[0]?.[0] as string);
    expect(url.pathname).toBe("/api/workspaces/w1/audit-events");
    expect(Object.fromEntries(url.searchParams)).toEqual({ action: "auth.login", format: "csv" });
  });

  it("reports the server's row cap", async () => {
    stubFetch(new Response("id\n", { status: 200, headers: { "X-Audit-Truncated": "true" } }));

    const result = await new ApiClient("https://api.test").exportWorkspaceAuditCsv("w1");

    expect(result.truncated).toBe(true);
  });

  it("throws the server's refusal", async () => {
    stubFetch(
      new Response(JSON.stringify({ error: "forbidden" }), {
        status: 403,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(new ApiClient("https://api.test").exportWorkspaceAuditCsv("w1")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("ApiClient audit list methods", () => {
  it("passes the cursor and parses the page", async () => {
    const fetchMock = stubFetch(
      new Response(JSON.stringify({ events: [], next_cursor: "c2" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const page = await new ApiClient("https://api.test").listWorkspaceAuditEvents("w1", {}, { limit: 50, cursor: "c1" });

    expect(page).toEqual({ events: [], next_cursor: "c2" });
    expect(new URL(fetchMock.mock.calls[0]?.[0] as string).search).toBe("?limit=50&cursor=c1");
  });

  it("returns an empty page for a malformed security events body", async () => {
    stubFetch(
      new Response(JSON.stringify({ events: "x" }), { status: 200, headers: { "Content-Type": "application/json" } }),
    );

    expect(await new ApiClient("https://api.test").listMySecurityEvents({ limit: 20 })).toEqual({
      events: [],
      next_cursor: null,
    });
  });
});
