// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { parseWithFallback, setSchemaLogger } from "./schema";
import { noopLogger } from "../logger";
import {
  MFAConfirmResponseSchema,
  MFAEnrollResponseSchema,
  PersonalAccessTokenListSchema,
  PersonalAccessTokenSchema,
  SSODomainListSchema,
  SSODomainSchema,
  WorkspacePolicySchema,
  DEFAULT_WORKSPACE_POLICY,
  NO_SSO_DOMAINS,
} from "./security-schemas";

const warn = vi.fn();
setSchemaLogger({ ...noopLogger, warn });
afterEach(() => warn.mockClear());

const parse = <T,>(data: unknown, schema: Parameters<typeof parseWithFallback>[1], fb: T) =>
  parseWithFallback<T>(data, schema, fb, { endpoint: "test" });

describe("WorkspacePolicySchema", () => {
  it("parses a full payload and keeps unknown keys", () => {
    const body = {
      allowed_invite_domains: ["a.test"],
      share_links_enabled: false,
      require_sso: true,
      require_mfa: true,
      session_max_age_hours: 24,
      updated_by: "u1",
      updated_at: "2026-01-01T00:00:00Z",
      future_flag: 1,
    };
    expect(parse(body, WorkspacePolicySchema, DEFAULT_WORKSPACE_POLICY)).toEqual(body);
    expect(warn).not.toHaveBeenCalled();
  });

  it("fills defaults for an empty object", () => {
    expect(parse({}, WorkspacePolicySchema, DEFAULT_WORKSPACE_POLICY)).toEqual(DEFAULT_WORKSPACE_POLICY);
    expect(DEFAULT_WORKSPACE_POLICY).toEqual({
      allowed_invite_domains: [],
      share_links_enabled: true,
      require_sso: false,
      require_mfa: false,
      session_max_age_hours: 0,
      updated_by: null,
      updated_at: null,
    });
  });

  it("turns null audit fields into null", () => {
    expect(parse({ updated_by: null, updated_at: null }, WorkspacePolicySchema, DEFAULT_WORKSPACE_POLICY)).toMatchObject({
      updated_by: null,
      updated_at: null,
    });
  });

  it.each([
    ["a list", []],
    ["a string domain list", { allowed_invite_domains: "a.test" }],
    ["a null domain list", { allowed_invite_domains: null }],
    ["a string boolean", { require_sso: "yes" }],
    ["a string age", { session_max_age_hours: "24" }],
    ["a null string", "null"],
  ])("falls back and warns for %s", (_n, body) => {
    expect(parse(body, WorkspacePolicySchema, DEFAULT_WORKSPACE_POLICY)).toEqual(DEFAULT_WORKSPACE_POLICY);
    expect(warn).toHaveBeenCalledTimes(1);
  });
});

describe("MFA schemas", () => {
  it("parses enroll and confirm responses", () => {
    expect(parse({ secret: "S", otpauth_uri: "otpauth://x" }, MFAEnrollResponseSchema, null)).toEqual({
      secret: "S",
      otpauth_uri: "otpauth://x",
    });
    expect(parse({ recovery_codes: ["a", "b"] }, MFAConfirmResponseSchema, null)).toEqual({ recovery_codes: ["a", "b"] });
  });
  it("rejects a confirm response without recovery codes", () => {
    expect(parse({}, MFAConfirmResponseSchema, null)).toBeNull();
  });
  it.each([
    [{ secret: 1, otpauth_uri: "x" }],
    [{ secret: "S" }],
    [{ secret: null, otpauth_uri: null }],
  ])("rejects a malformed enroll response %#", (body) => {
    expect(parse(body, MFAEnrollResponseSchema, null)).toBeNull();
    expect(warn).toHaveBeenCalledTimes(1);
  });
  it("rejects recovery codes that are not strings", () => {
    expect(parse({ recovery_codes: [1] }, MFAConfirmResponseSchema, null)).toBeNull();
    expect(warn).toHaveBeenCalledTimes(1);
  });
});

describe("SSO domain schemas", () => {
  const domain = { domain: "a.test", txt_name: "_multica.a.test", txt_value: "v", verified_at: null };
  it("parses a domain and a list", () => {
    expect(parse(domain, SSODomainSchema, null)).toEqual(domain);
    expect(parse({ ...domain, verified_at: "2026-01-01T00:00:00Z", extra: 1 }, SSODomainSchema, null)).toMatchObject({
      verified_at: "2026-01-01T00:00:00Z",
      extra: 1,
    });
    expect(parse({ domains: [domain] }, SSODomainListSchema, NO_SSO_DOMAINS)).toEqual({ domains: [domain] });
  });
  it("defaults a missing domains array and a missing verified_at", () => {
    expect(parse({}, SSODomainListSchema, NO_SSO_DOMAINS)).toEqual({ domains: [] });
    expect(parse({ domain: "a.test", txt_name: "n", txt_value: "v" }, SSODomainSchema, null)).toMatchObject({ verified_at: null });
  });
  it.each([
    ["a null domain", { ...domain, domain: null }],
    ["a numeric txt value", { ...domain, txt_value: 1 }],
    ["a missing domain", { txt_name: "n", txt_value: "v" }],
  ])("falls back for %s", (_n, body) => {
    expect(parse(body, SSODomainSchema, null)).toBeNull();
    expect(warn).toHaveBeenCalledTimes(1);
  });
  it("falls back for a list whose domains is not an array", () => {
    expect(parse({ domains: "x" }, SSODomainListSchema, NO_SSO_DOMAINS)).toEqual({ domains: [] });
    expect(warn).toHaveBeenCalledTimes(1);
  });
});

describe("personal access token schemas", () => {
  const pat = {
    id: "t1",
    name: "ci",
    token_prefix: "mul_ab",
    expires_at: null,
    last_used_at: null,
    created_at: "2026-01-01T00:00:00Z",
    workspace_id: "w1",
    read_only: true,
  };
  it("parses a token and defaults the optional fields", () => {
    expect(parse(pat, PersonalAccessTokenSchema, null)).toEqual(pat);
    expect(
      parse({ id: "t1", name: "ci", token_prefix: "p", created_at: "c" }, PersonalAccessTokenSchema, null),
    ).toMatchObject({ workspace_id: null, read_only: false, expires_at: null, last_used_at: null });
  });
  it("keeps unknown keys and the create response token", () => {
    expect(parse({ ...pat, token: "mul_secret", scopes: [] }, PersonalAccessTokenSchema, null)).toMatchObject({
      token: "mul_secret",
      scopes: [],
    });
  });
  it.each([[{ ...pat, id: 5 }], [{ ...pat, read_only: "yes" }], [{ ...pat, name: null }]])(
    "rejects a malformed token %#",
    (body) => {
      expect(parse(body, PersonalAccessTokenSchema, null)).toBeNull();
      expect(warn).toHaveBeenCalledTimes(1);
    },
  );
  it("falls back to [] for a malformed list", () => {
    expect(parse([pat], PersonalAccessTokenListSchema, [])).toEqual([pat]);
    expect(parse({ tokens: [] }, PersonalAccessTokenListSchema, [])).toEqual([]);
    expect(warn).toHaveBeenCalledTimes(1);
  });
});
