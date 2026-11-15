import { api } from "../api";
import { ApiError } from "../api/client";

export const ACCOUNT_EXPORT_RATE_LIMITED = "export_rate_limited";

/** The export was already downloaded within the last hour. */
export class AccountExportRateLimitedError extends Error {
  readonly code = ACCOUNT_EXPORT_RATE_LIMITED;

  constructor(message = "Account export is limited to once an hour") {
    super(message);
    this.name = "AccountExportRateLimitedError";
  }
}

/**
 * Fetches the account export as a Blob for the caller to save. A 429 becomes
 * an `AccountExportRateLimitedError` whose `code` is `export_rate_limited`.
 */
export async function downloadAccountExport(): Promise<Blob> {
  try {
    return await api.exportAccount();
  } catch (err) {
    if (err instanceof ApiError && err.status === 429) {
      throw new AccountExportRateLimitedError(err.message);
    }
    throw err;
  }
}
