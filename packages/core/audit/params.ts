/** Filters of the workspace audit list. Timestamps are RFC 3339. */
export interface AuditFilters {
  action?: string;
  actorId?: string;
  /** Inclusive. */
  from?: string;
  /** Exclusive. */
  to?: string;
}

export interface AuditPageParams {
  limit?: number;
  cursor?: string | null;
}

/** Builds the query string, leaving out every parameter that is unset or empty. */
export function auditQueryString(
  filters: AuditFilters = {},
  page: AuditPageParams = {},
  format?: "csv",
): string {
  const params = new URLSearchParams();
  if (filters.action) params.set("action", filters.action);
  if (filters.actorId) params.set("actor_id", filters.actorId);
  if (filters.from) params.set("from", filters.from);
  if (filters.to) params.set("to", filters.to);
  if (page.limit !== undefined) params.set("limit", String(page.limit));
  if (page.cursor) params.set("cursor", page.cursor);
  if (format) params.set("format", format);
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}
