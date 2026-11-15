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
