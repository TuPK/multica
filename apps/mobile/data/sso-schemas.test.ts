// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { parseWithFallback } from "@/lib/parse-response";
import {
  EMPTY_SSO_DISCOVERY,
  EMPTY_SSO_EXCHANGE,
  SsoDiscoverySchema,
  SsoExchangeSchema,
} from "./schemas";

afterEach(() => vi.restoreAllMocks());

const parseDiscovery = (raw: unknown) =>
  parseWithFallback(raw, SsoDiscoverySchema, EMPTY_SSO_DISCOVERY, { endpoint: "discover" });
const parseExchange = (raw: unknown) =>
  parseWithFallback(raw, SsoExchangeSchema, EMPTY_SSO_EXCHANGE, { endpoint: "exchange" });

describe("SSO discover response", () => {
  it("reads an SSO answer and tolerates extra fields", () => {
    expect(parseDiscovery({ sso: true, start_url: "https://x/start", extra: 1 })).toMatchObject({
      sso: true,
      start_url: "https://x/start",
    });
    expect(parseDiscovery({ sso: false })).toEqual({ sso: false, start_url: "" });
  });

  it.each([null, "oops", 42, [], { sso: "yes" }])("falls back to no SSO for %j", (raw) => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(parseDiscovery(raw)).toEqual({ sso: false, start_url: "" });
  });
});

describe("SSO exchange response", () => {
  const user = { id: "u1", name: "Alex", email: "alex@example.com" };

  it("reads token and user like verify-code", () => {
    const parsed = parseExchange({ token: "jwt", user, extra: true });
    expect(parsed.token).toBe("jwt");
    expect(parsed.user).toMatchObject({ id: "u1", email: "alex@example.com" });
  });

  it.each([null, "oops", {}, { token: "", user }, { token: "jwt" }, { token: "jwt", user: {} }])(
    "falls back to an empty token for %j",
    (raw) => {
      vi.spyOn(console, "warn").mockImplementation(() => {});
      expect(parseExchange(raw).token).toBe("");
    },
  );
});
