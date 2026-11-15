// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { PlatformApiError, platformFetch } from "./client";

function mockFetch(response: Response) {
  const fn = vi.fn<typeof fetch>().mockResolvedValue(response);
  vi.stubGlobal("fetch", fn);
  return fn;
}

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

async function catchError(promise: Promise<unknown>): Promise<PlatformApiError> {
  try {
    await promise;
  } catch (err) {
    expect(err).toBeInstanceOf(PlatformApiError);
    return err as PlatformApiError;
  }
  throw new Error("expected platformFetch to reject");
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("platformFetch request", () => {
  it("calls the path under /api/platform with same-origin credentials", async () => {
    const fn = mockFetch(json(200, { ok: true }));
    await platformFetch("/tenants?q=acme");
    const [url, init] = fn.mock.calls[0]!;
    expect(url).toBe("/api/platform/tenants?q=acme");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("Accept")).toBe("application/json");
  });

  it("sends a body as JSON with a content type, and no body otherwise", async () => {
    const fn = mockFetch(json(200, {}));
    await platformFetch("/tenants/1/status", { method: "POST", body: { status: "suspended", reason: "abuse" } });
    const [, withBody] = fn.mock.calls[0]!;
    expect(withBody?.method).toBe("POST");
    expect(withBody?.body).toBe(JSON.stringify({ status: "suspended", reason: "abuse" }));
    expect(new Headers(withBody?.headers).get("Content-Type")).toBe("application/json");

    fn.mockResolvedValue(json(200, {}));
    await platformFetch("/overview");
    const [, withoutBody] = fn.mock.calls[1]!;
    expect(withoutBody?.body).toBeUndefined();
    expect(new Headers(withoutBody?.headers).has("Content-Type")).toBe(false);
  });

  it("does not set a cookie or credential header itself", async () => {
    const fn = mockFetch(json(200, {}));
    await platformFetch("/overview", { headers: { "X-Test": "1" } });
    const headers = new Headers(fn.mock.calls[0]![1]?.headers);
    expect(headers.has("Cookie")).toBe(false);
    expect(headers.has("Authorization")).toBe(false);
    expect(headers.get("X-Test")).toBe("1");
  });
});

describe("platformFetch response", () => {
  it("returns the decoded JSON body", async () => {
    mockFetch(json(200, { user_id: "u1", role: "admin" }));
    await expect(platformFetch("/me")).resolves.toEqual({ user_id: "u1", role: "admin" });
  });

  it("returns null for an empty 204 answer", async () => {
    mockFetch(new Response(null, { status: 204 }));
    await expect(platformFetch("/auth/logout", { method: "POST" })).resolves.toBeNull();
  });

  it("throws a 2xx body that is not JSON as malformed_response", async () => {
    mockFetch(new Response("<html></html>", { status: 200 }));
    const err = await catchError(platformFetch("/me"));
    expect(err.status).toBe(200);
    expect(err.code).toBe("malformed_response");
  });
});

describe("platformFetch error mapping", () => {
  it("maps status, code and message from the error body", async () => {
    mockFetch(json(400, { error: "the last active administrator cannot be removed", code: "last_admin" }));
    const err = await catchError(platformFetch("/staff/u1", { method: "DELETE" }));
    expect(err.status).toBe(400);
    expect(err.code).toBe("last_admin");
    expect(err.message).toBe("the last active administrator cannot be removed");
    expect(err.name).toBe("PlatformApiError");
  });

  it("uses a null code when the body has none", async () => {
    mockFetch(json(500, { error: "internal error" }));
    const err = await catchError(platformFetch("/overview"));
    expect(err.status).toBe(500);
    expect(err.code).toBeNull();
    expect(err.message).toBe("internal error");
  });

  it("ignores a code that is not a string", async () => {
    mockFetch(json(400, { error: "bad", code: 7 }));
    const err = await catchError(platformFetch("/overview"));
    expect(err.code).toBeNull();
  });

  it("maps a plain-text 404 to a null code and a generic message", async () => {
    mockFetch(new Response("404 page not found\n", { status: 404 }));
    const err = await catchError(platformFetch("/me"));
    expect(err.status).toBe(404);
    expect(err.code).toBeNull();
    expect(err.message).toBe("Request failed with status 404");
  });

  it("maps an error body that is a JSON scalar or null", async () => {
    for (const body of ["null", "7", '"text"']) {
      mockFetch(new Response(body, { status: 502 }));
      const err = await catchError(platformFetch("/overview"));
      expect(err.status).toBe(502);
      expect(err.code).toBeNull();
    }
  });

  it("maps an empty error body", async () => {
    mockFetch(new Response(null, { status: 403 }));
    const err = await catchError(platformFetch("/overview"));
    expect(err.status).toBe(403);
    expect(err.code).toBeNull();
  });
});
