// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api-error";
import { mapAuthError } from "./auth-error";

const t = (key: string) => `t:${key}`;

describe("mapAuthError", () => {
  it("maps structured MFA and session error codes without inspecting English messages", () => {
    expect(
      mapAuthError(
        new ApiError("400 Bad Request", 400, { code: "invalid_code", error: "wrong" }),
        "fallback",
        t,
      ),
    ).toBe("t:mfa.invalid_code");

    expect(
      mapAuthError(
        new ApiError("401 Unauthorized", 401, { code: "mfa_token_invalid" }),
        "fallback",
        t,
      ),
    ).toBe("t:mfa.token_invalid");

    expect(
      mapAuthError(
        new ApiError("401 Unauthorized", 401, { code: "mfa_token_failed" }),
        "fallback",
        t,
      ),
    ).toBe("t:mfa.token_failed");

    expect(
      mapAuthError(
        new ApiError("401 Unauthorized", 401, { code: "session_revoked" }),
        "fallback",
        t,
      ),
    ).toBe("t:reasons.session_revoked");

    expect(
      mapAuthError(
        new ApiError("403 Forbidden", 403, { code: "account_suspended" }),
        "fallback",
        t,
      ),
    ).toBe("t:reasons.account_suspended");
  });

  it("maps 429 and 400 status codes without reading body.error English text", () => {
    expect(
      mapAuthError(new ApiError("429 Too Many Requests", 429), "fallback", t),
    ).toBe("t:verify.rate_limited");
    expect(
      mapAuthError(
        new ApiError("400 Bad Request", 400, { error: "some server text" }),
        "fallback",
        t,
      ),
    ).toBe("t:verify.invalid_code");
  });

  it("falls back cleanly for unrecognized ApiErrors and maps network errors", () => {
    expect(
      mapAuthError(new ApiError("500 Server Error", 500), "fallback", t),
    ).toBe("fallback");
    expect(mapAuthError(new TypeError("Network request failed"), "fallback", t)).toBe(
      "t:verify.network",
    );
  });
});
