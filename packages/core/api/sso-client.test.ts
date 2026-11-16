import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

function respondWith(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(status === 204 ? null : typeof body === "string" ? body : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const client = () => new ApiClient("https://api.example.test");

const connection = {
  id: "c1",
  workspace_id: "w1",
  issuer: "https://idp.example.test",
  client_id: "client",
  client_secret_set: true,
  default_role: "member",
  status: "active",
  redirect_uri: "https://api.example.test/auth/sso/callback",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("discoverSSO", () => {
  it("returns the start url for an SSO domain", async () => {
    const fetchMock = respondWith(200, { sso: true, start_url: "https://api.example.test/auth/sso/start?connection=c1" });
    await expect(client().discoverSSO("a@example.test")).resolves.toEqual({
      sso: true,
      start_url: "https://api.example.test/auth/sso/start?connection=c1",
    });
    expect(fetchMock.mock.calls[0]![0]).toBe("https://api.example.test/auth/sso/discover");
    expect(JSON.parse((fetchMock.mock.calls[0]![1] as RequestInit).body as string)).toEqual({ email: "a@example.test" });
  });

  it("answers no for a plain domain", async () => {
    respondWith(200, { sso: false });
    await expect(client().discoverSSO("a@example.test")).resolves.toEqual({ sso: false, start_url: null });
  });

  it.each([
    ["a body that is not an object", "[]"],
    ["sso as a string", { sso: "yes", start_url: "/x" }],
    ["sso true with no start url", { sso: true }],
    ["sso true with a null start url", { sso: true, start_url: null }],
    ["a wrong start url type", { sso: true, start_url: 5 }],
  ])("answers no for %s, so the sign-in page keeps its other methods", async (_name, body) => {
    respondWith(200, body);
    await expect(client().discoverSSO("a@example.test")).resolves.toEqual({ sso: false, start_url: null });
  });

  it("answers no when the call fails", async () => {
    respondWith(500, { error: "boom" });
    await expect(client().discoverSSO("a@example.test")).resolves.toEqual({ sso: false, start_url: null });
  });
});

describe("SSO connection", () => {
  it("parses a connection and keeps unknown fields from a newer server", async () => {
    respondWith(200, { ...connection, scopes: ["openid"], status: "paused-by-newer-server" });
    const got = await client().getSSOConnection("w1");
    expect(got).toMatchObject({ id: "c1", client_secret_set: true, status: "paused-by-newer-server" });
  });

  it("fills defaults for optional fields", async () => {
    respondWith(200, { id: "c1", issuer: "https://idp.example.test", client_id: "client" });
    await expect(client().getSSOConnection("w1")).resolves.toMatchObject({
      client_secret_set: false,
      default_role: "member",
      status: "active",
      redirect_uri: "",
    });
  });

  it("reads 404 as no connection and rethrows other errors", async () => {
    respondWith(404, { error: "none", code: "sso_connection_not_found" });
    await expect(client().getSSOConnection("w1")).resolves.toBeNull();
    respondWith(403, { error: "no" });
    await expect(client().getSSOConnection("w1")).rejects.toBeInstanceOf(ApiError);
  });

  it.each([
    ["no id", { issuer: "https://idp.example.test", client_id: "client" }],
    ["an empty id", { ...connection, id: "" }],
    ["a numeric issuer", { ...connection, issuer: 5 }],
    ["a string client_secret_set", { ...connection, client_secret_set: "true" }],
    ["a list", [connection]],
  ])("returns null for a malformed body: %s", async (_name, body) => {
    respondWith(200, body);
    await expect(client().getSSOConnection("w1")).resolves.toBeNull();
    respondWith(200, body);
    await expect(client().putSSOConnection("w1", { issuer: "https://idp.example.test", client_id: "client" })).resolves.toBeNull();
  });

  it("sends the input on PUT and surfaces the error code", async () => {
    const fetchMock = respondWith(201, connection);
    await client().putSSOConnection("w1", { issuer: connection.issuer, client_id: "client", client_secret: "s3cret", default_role: "admin" });
    const init = fetchMock.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({ issuer: connection.issuer, client_id: "client", client_secret: "s3cret", default_role: "admin" });

    respondWith(422, { error: "verify a domain first", code: "domain_not_verified" });
    const err = await client().putSSOConnection("w1", { issuer: connection.issuer, client_id: "client" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(422);
    expect(((err as ApiError).body as { code: string }).code).toBe("domain_not_verified");
  });

  it("deletes the connection", async () => {
    const fetchMock = respondWith(204, null);
    await expect(client().deleteSSOConnection("w1")).resolves.toBeUndefined();
    expect((fetchMock.mock.calls[0]![1] as RequestInit).method).toBe("DELETE");
  });
});
