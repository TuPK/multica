import { ApiError, extractErrorCode } from "./api-error";

/**
 * Map backend auth errors to localized strings by `code` (never by server
 * English message) and HTTP status / network error type, falling back to
 * the caller's default for unrecognised shapes.
 */
type Translate = (key: string) => string;

export function mapAuthError(err: unknown, fallback: string, t?: Translate): string {
  const translate = (key: string, enDefault: string) => (t ? t(key) : enDefault);

  if (err instanceof ApiError) {
    const code = err.code ?? extractErrorCode(err.body);
    switch (code) {
      case "invalid_code":
        return translate("mfa.invalid_code", "Invalid code. Try again.");
      case "mfa_token_invalid":
        return translate(
          "mfa.token_invalid",
          "Verification expired. Sign in again.",
        );
      case "mfa_token_failed":
        return translate(
          "mfa.token_failed",
          "Too many attempts. Sign in again.",
        );
      case "session_revoked":
        return translate(
          "reasons.session_revoked",
          "Your session ended. Sign in again.",
        );
      case "account_suspended":
        return translate(
          "reasons.account_suspended",
          "Your account is suspended.",
        );
      default:
        break;
    }
    if (err.status === 429) {
      return translate(
        "verify.rate_limited",
        "Too many attempts. Wait a moment and try again.",
      );
    }
    if (err.status === 400) {
      return translate(
        "verify.invalid_code",
        "That code isn't valid or has expired.",
      );
    }
    return fallback;
  }

  if (err instanceof Error) {
    if (
      err.name === "AbortError" ||
      err instanceof TypeError ||
      /network|fetch|timeout|unreachable/i.test(err.message)
    ) {
      return translate(
        "verify.network",
        "Can't reach Delegora. Check your connection and retry.",
      );
    }
  }

  return fallback;
}
