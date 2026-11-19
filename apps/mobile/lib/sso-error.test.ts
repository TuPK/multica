// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ssoErrorKey } from "./sso-error";

describe("ssoErrorKey", () => {
  it.each([
    ["domain_not_allowed", "sso.domain_not_allowed"],
    ["email_not_verified", "sso.email_not_verified"],
    ["signup_not_allowed", "sso.signup_not_allowed"],
    ["account_unavailable", "sso.account_unavailable"],
    ["seat_capacity_unavailable", "sso.seat_capacity_unavailable"],
    ["sso_unavailable", "sso.unavailable"],
    ["sso_provider_unavailable", "sso.unavailable"],
  ])("maps %s", (code, key) => {
    expect(ssoErrorKey(code)).toBe(key);
  });

  it.each(["sso_login_failed", "sso_required", "something_new", "", undefined, "constructor"])(
    "uses the generic message for %j",
    (code) => {
      expect(ssoErrorKey(code)).toBe("sso.failed");
    },
  );
});
