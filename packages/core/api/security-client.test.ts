// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";

afterEach(() => vi.unstubAllGlobals());

function respondWith(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(status === 204 ? null : JSON.stringify(body), {
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
  method: ((m.mock.calls[0]![1] as RequestInit).method ?? "GET") as string,
  body: (m.mock.calls[0]![1] as RequestInit).body
    ? JSON.parse((m.mock.calls[0]![1] as RequestInit).body as string)
    : undefined,
});
const codeOf = (e: unknown) => ((e as ApiError).body as { code: string }).code;

const policy = {
  allowed_invite_domains: ["a.test"],
  share_links_enabled: false,
  require_sso: true,
  require_mfa: false,
  session_max_age_hours: 12,
  updated_by: "u1",
  updated_at: "2026-01-01T00:00:00Z",
};
const dom = { domain: "a.test", txt_name: "_multica.a.test", txt_value: "v", verified_at: null };
const pat = { id: "t1", name: "ci", token_prefix: "mul_", expires_at: null, last_used_at: null, created_at: "c" };

describe("workspace policy", () => {
  it("gets the policy", async () => {
    const f = respondWith(200, policy);
    await expect(client().getWorkspacePolicy("w1")).resolves.toEqual(policy);
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/workspaces/w1/policy", method: "GET" });
  });
  it("falls back to the defaults for a malformed body", async () => {
    respondWith(200, { require_mfa: "yes" });
    await expect(client().getWorkspacePolicy("w1")).resolves.toMatchObject({
      share_links_enabled: true,
      require_mfa: false,
      allowed_invite_domains: [],
      session_max_age_hours: 0,
    });
  });
  it("puts a partial input and keeps the error code", async () => {
    const f = respondWith(200, policy);
    await client().updateWorkspacePolicy("w1", { require_sso: true });
    expect(call(f)).toMatchObject({ method: "PUT", url: "https://api.example.test/api/workspaces/w1/policy", body: { require_sso: true } });
    respondWith(422, { error: "x", code: "invite_domain_not_allowed" });
    expect(codeOf(await client().updateWorkspacePolicy("w1", {}).catch((e) => e))).toBe("invite_domain_not_allowed");
    respondWith(402, { error: "x", code: "plan_feature_unavailable" });
    const err = await client().updateWorkspacePolicy("w1", {}).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(402);
    expect(codeOf(err)).toBe("plan_feature_unavailable");
  });
});

describe("MFA", () => {
  it("enrolls", async () => {
    const f = respondWith(200, { secret: "S", otpauth_uri: "otpauth://x" });
    await expect(client().enrollMFA()).resolves.toEqual({ secret: "S", otpauth_uri: "otpauth://x" });
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/me/mfa/enroll", method: "POST" });
  });
  it("lets a 409 propagate", async () => {
    respondWith(409, { error: "x", code: "mfa_already_enabled" });
    expect(codeOf(await client().enrollMFA().catch((e) => e))).toBe("mfa_already_enabled");
  });
  it("confirms with the code", async () => {
    const f = respondWith(200, { recovery_codes: ["r1"] });
    await expect(client().confirmMFA("123456")).resolves.toEqual({ recovery_codes: ["r1"] });
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/me/mfa/confirm", method: "POST", body: { code: "123456" } });
    respondWith(200, { recovery_codes: "no" });
    await expect(client().confirmMFA("1")).rejects.toThrow(/no recovery codes/);
  });
  it("disables with a code and returns void", async () => {
    const f = respondWith(204, null);
    await expect(client().disableMFA("abcd-efgh")).resolves.toBeUndefined();
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/me/mfa/disable", method: "POST", body: { code: "abcd-efgh" } });
  });
});

describe("SSO domains", () => {
  it("lists, and falls back to an empty list", async () => {
    const f = respondWith(200, { domains: [dom] });
    await expect(client().listSSODomains("w1")).resolves.toEqual({ domains: [dom] });
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/workspaces/w1/sso/domains", method: "GET" });
    respondWith(200, { domains: 5 });
    await expect(client().listSSODomains("w1")).resolves.toEqual({ domains: [] });
  });
  it("adds", async () => {
    const f = respondWith(201, dom);
    await expect(client().addSSODomain("w1", "a.test")).resolves.toEqual(dom);
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/workspaces/w1/sso/domains", method: "POST", body: { domain: "a.test" } });
  });
  it("verifies, encodes the domain and keeps the code", async () => {
    const f = respondWith(200, { ...dom, verified_at: "2026-01-01T00:00:00Z" });
    await client().verifySSODomain("w1", "a.test");
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/workspaces/w1/sso/domains/a.test/verify", method: "POST" });
    respondWith(422, { error: "x", code: "domain_not_verified" });
    expect(codeOf(await client().verifySSODomain("w1", "a.test").catch((e) => e))).toBe("domain_not_verified");
  });
  it("removes", async () => {
    const f = respondWith(204, null);
    await expect(client().removeSSODomain("w1", "a.test")).resolves.toBeUndefined();
    expect(call(f)).toMatchObject({ url: "https://api.example.test/api/workspaces/w1/sso/domains/a.test", method: "DELETE" });
  });
});

describe("personal access tokens", () => {
  it("lists with defaults and falls back to []", async () => {
    respondWith(200, [pat]);
    await expect(client().listPersonalAccessTokens()).resolves.toEqual([{ ...pat, workspace_id: null, read_only: false }]);
    respondWith(200, { nope: true });
    await expect(client().listPersonalAccessTokens()).resolves.toEqual([]);
  });
  it("creates and returns the token", async () => {
    const f = respondWith(201, { ...pat, token: "mul_secret", read_only: true, workspace_id: "w1" });
    const got = await client().createPersonalAccessToken({ name: "ci", workspace_id: "w1", read_only: true });
    expect(got).toMatchObject({ token: "mul_secret", read_only: true, workspace_id: "w1" });
    expect(call(f).body).toEqual({ name: "ci", workspace_id: "w1", read_only: true });
  });
  it("throws when the create response has no token", async () => {
    respondWith(201, pat);
    await expect(client().createPersonalAccessToken({ name: "ci" })).rejects.toThrow();
  });
});
