import { ApiError } from "./api-error";

/**
 * Shared TanStack Query retry policy.
 *
 * Retries transient failures once (`failureCount < 1`), but never retries
 * 401/403 responses so revoked sessions, suspended accounts, and blocked
 * workspaces do not enter retry loops.
 */
export function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
    return false;
  }
  return failureCount < 1;
}
