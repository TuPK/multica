// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";

afterEach(() => vi.unstubAllGlobals());

function respondWith(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
const client = () => new ApiClient("https://api.example.test");
const call = (m: ReturnType<typeof vi.fn>) => ({
  url: m.mock.calls[0]![0] as string,
  method: (m.mock.calls[0]![1] as RequestInit).method as string,
  body: JSON.parse((m.mock.calls[0]![1] as RequestInit).body as string),
});
const codeOf = (e: unknown) => ((e as ApiError).body as { code: string }).code;
const user = { id: "u1", name: "Alice", email: "a@x.test", avatar_url: null };

describe("attemptLoginCode", () => {
  it("returns a session", async () => {
    const f = respondWith(200, { token: "t", user });
    const r = await client().attemptLoginCode("a@x.test", "123456");
    expect(r).toMatchObject({ kind: "session", token: "t", user: { id: "u1" } });
    expect(call(f)).toMatchObject({
      url: "https://api.example.test/auth/verify-code",
      method: "POST",
      body: { email: "a@x.test", code: "123456" },
    });
  });
  it("returns an mfa challenge", async () => {
    respondWith(200, { mfa_required: true, mfa_token: "m", expires_in: 300 });
    await expect(client().attemptLoginCode("a@x.test", "123456")).resolves.toEqual({
      kind: "mfa",
      mfaToken: "m",
      expiresIn: 300,
    });
  });
  it("throws on a malformed answer", async () => {
    respondWith(200, { user });
    await expect(client().attemptLoginCode("a@x.test", "1")).rejects.toThrow(/Malformed/);
    respondWith(200, { mfa_required: true, expires_in: 300 });
    await expect(client().attemptLoginCode("a@x.test", "1")).rejects.toThrow(/Malformed/);
  });
  it("keeps the error code", async () => {
    respondWith(400, { error: "x", code: "invalid_code" });
    expect(codeOf(await client().attemptLoginCode("a@x.test", "1").catch((e) => e))).toBe("invalid_code");
  });
});

describe("verifyMFALogin", () => {
  it("posts the token and code and returns the session", async () => {
    const f = respondWith(200, { token: "t", user });
    await expect(client().verifyMFALogin("m", "123456")).resolves.toMatchObject({ token: "t", user: { id: "u1" } });
    expect(call(f)).toMatchObject({
      url: "https://api.example.test/auth/mfa/verify",
      method: "POST",
      body: { mfa_token: "m", code: "123456" },
    });
  });
  it("keeps mfa_token_invalid and invalid_code", async () => {
    respondWith(401, { error: "x", code: "mfa_token_invalid" });
    expect(codeOf(await client().verifyMFALogin("m", "1").catch((e) => e))).toBe("mfa_token_invalid");
    respondWith(400, { error: "x", code: "invalid_code" });
    expect(codeOf(await client().verifyMFALogin("m", "1").catch((e) => e))).toBe("invalid_code");
  });
  it("throws on a malformed answer", async () => {
    respondWith(200, { token: "t" });
    await expect(client().verifyMFALogin("m", "1")).rejects.toThrow(/Malformed/);
  });
});
