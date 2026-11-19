const KEYS: Record<string, string> = {
  domain_not_allowed: "sso.domain_not_allowed",
  email_not_verified: "sso.email_not_verified",
  signup_not_allowed: "sso.signup_not_allowed",
  account_unavailable: "sso.account_unavailable",
  seat_capacity_unavailable: "sso.seat_capacity_unavailable",
  sso_unavailable: "sso.unavailable",
  sso_provider_unavailable: "sso.unavailable",
};

/** auth-namespace copy key for an `error` code on the sso-complete deep link. */
export function ssoErrorKey(code: string | undefined): string {
  return (code && Object.hasOwn(KEYS, code) ? KEYS[code] : undefined) ?? "sso.failed";
}
