import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError, errorCode } from "../api/client";

afterEach(() => {
  vi.unstubAllGlobals();
});

function respond(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("ApiClient.revokeAllSessions", () => {
  it("POSTs the endpoint and parses the response", async () => {
    const fetchMock = respond({ revoked_at: "2026-11-15T06:30:00Z", token: "new-jwt" });

    const result = await new ApiClient("https://api.test").revokeAllSessions();

    expect(result).toEqual({ revoked_at: "2026-11-15T06:30:00Z", token: "new-jwt" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.test/api/me/sessions/revoke-all");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
  });

  it("returns the empty response for a malformed body instead of throwing", async () => {
    respond({ token: 7, revoked_at: [] });

    expect(await new ApiClient("https://api.test").revokeAllSessions()).toEqual({
      revoked_at: "",
      token: "",
    });
  });

  it("surfaces a server refusal with its code", async () => {
    respond({ error: "forbidden", code: "account_suspended" }, 403);

    const err = await new ApiClient("https://api.test").revokeAllSessions().catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(errorCode(err)).toBe("account_suspended");
  });
});

describe("ApiClient.acceptConsents", () => {
  it("POSTs the documents and parses what remains", async () => {
    const fetchMock = respond({ consent_required: [{ document: "privacy", version: "3", url: "https://x.test/p" }] });

    const result = await new ApiClient("https://api.test").acceptConsents([{ document: "terms", version: "2" }]);

    expect(result.consent_required).toHaveLength(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.test/api/me/consents");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ documents: [{ document: "terms", version: "2" }] });
  });

  it("falls back to nothing outstanding for a malformed body", async () => {
    respond({ consent_required: 5 });

    expect(await new ApiClient("https://api.test").acceptConsents([])).toEqual({ consent_required: [] });
  });

  it("surfaces a version mismatch with its code", async () => {
    respond({ error: "stale", code: "consent_version_mismatch" }, 409);

    const err = await new ApiClient("https://api.test").acceptConsents([]).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(errorCode(err)).toBe("consent_version_mismatch");
  });
});

describe("ApiClient.exportAccount", () => {
  it("GETs the export as a Blob", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{"exported_at":"x"}', { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const blob = await new ApiClient("https://api.test").exportAccount();

    expect(await blob.text()).toBe('{"exported_at":"x"}');
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.test/api/me/export");
  });
});
