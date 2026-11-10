/** Share of a limit in use from which the meter warns. */
export const HUB_QUOTA_WARNING_RATIO = 0.8;

/**
 * unlimited: the limit is 0 or less, so only the usage is shown.
 * ok: below the warning share. warning: at or above the warning share.
 * full: at or over the limit, where new items or bytes are refused.
 */
export type HubQuotaLevel = "unlimited" | "ok" | "warning" | "full";

export function hubQuotaLevel(used: number, limit: number): HubQuotaLevel {
  if (limit <= 0) return "unlimited";
  if (used >= limit) return "full";
  if (used / limit >= HUB_QUOTA_WARNING_RATIO) return "warning";
  return "ok";
}
