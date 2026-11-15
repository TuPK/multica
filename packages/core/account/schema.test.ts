import { describe, expect, it } from "vitest";
import { parseWithFallback } from "../api/schema";
import {
  EMPTY_REVOKE_ALL_SESSIONS_RESPONSE,
  RevokeAllSessionsResponseSchema,
  type RevokeAllSessionsResponse,
} from "./schema";

function parse(raw: unknown): RevokeAllSessionsResponse {
  return parseWithFallback<RevokeAllSessionsResponse>(
    raw,
    RevokeAllSessionsResponseSchema,
    EMPTY_REVOKE_ALL_SESSIONS_RESPONSE,
    { endpoint: "POST /api/me/sessions/revoke-all" },
  );
}

describe("RevokeAllSessionsResponseSchema", () => {
  it("reads the server's body", () => {
    expect(parse({ revoked_at: "2026-11-15T06:30:00Z", token: "jwt" })).toEqual({
      revoked_at: "2026-11-15T06:30:00Z",
      token: "jwt",
    });
  });

  it("keeps fields a newer server adds", () => {
    expect(parse({ revoked_at: "t", token: "jwt", devices: 3 })).toMatchObject({ devices: 3 });
  });

  it("defaults a missing field instead of failing", () => {
    expect(parse({ revoked_at: "t" })).toEqual({ revoked_at: "t", token: "" });
  });

  it.each([null, undefined, "ok", 42, [], { token: 7 }, { revoked_at: {} }])(
    "falls back to an empty response for a malformed body: %j",
    (raw) => {
      expect(parse(raw)).toEqual(EMPTY_REVOKE_ALL_SESSIONS_RESPONSE);
    },
  );

  it("never invents a token for a malformed body", () => {
    expect(parse({ token: false }).token).toBe("");
  });
});
