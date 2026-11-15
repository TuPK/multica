import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { AuditFilters } from "./params";

export const AUDIT_PAGE_SIZE = 50;
export const SECURITY_EVENTS_PAGE_SIZE = 20;

export const auditKeys = {
  all: (wsId: string) => ["workspaces", wsId, "audit-events"] as const,
  list: (wsId: string, filters: AuditFilters) =>
    [
      ...auditKeys.all(wsId),
      "list",
      {
        action: filters.action ?? null,
        actorId: filters.actorId ?? null,
        from: filters.from ?? null,
        to: filters.to ?? null,
      },
    ] as const,
  // Account-level, so it carries the user and no workspace.
  securityEvents: (userId: string, limit: number) =>
    ["account", userId, "security-events", { limit }] as const,
};

/** Workspace audit log, newest first, one page per cursor. */
export const auditEventsOptions = (wsId: string, filters: AuditFilters = {}) =>
  infiniteQueryOptions({
    queryKey: auditKeys.list(wsId, filters),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) =>
      api.listWorkspaceAuditEvents(wsId, filters, {
        limit: AUDIT_PAGE_SIZE,
        cursor: pageParam,
      }),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: !!wsId,
  });

/** The user's most recent account-level events. */
export const securityEventsOptions = (
  userId: string,
  limit: number = SECURITY_EVENTS_PAGE_SIZE,
) =>
  queryOptions({
    queryKey: auditKeys.securityEvents(userId, limit),
    queryFn: () => api.listMySecurityEvents({ limit }),
    enabled: !!userId,
  });
