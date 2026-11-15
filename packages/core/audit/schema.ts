import { z } from "zod";

/**
 * One audit event as the server returns it. `action` is a plain string: the
 * server may add actions a client does not know, and the UI shows the code as
 * it is. `workspace_id` is null for account-level events and `actor_id` is null
 * for system actors.
 */
export interface AuditEvent {
  id: string;
  workspace_id: string | null;
  actor_type: string;
  actor_id: string | null;
  actor_label: string;
  action: string;
  target_type: string;
  target_id: string;
  target_label: string;
  detail: Record<string, unknown>;
  ip: string;
  user_agent: string;
  request_id: string;
  created_at: string;
}

/** One page of GET /api/workspaces/{id}/audit-events or /api/me/security-events. */
export interface AuditEventPage {
  events: AuditEvent[];
  next_cursor: string | null;
}

export const AuditEventSchema = z
  .object({
    id: z.string(),
    workspace_id: z.string().nullable().default(null),
    actor_type: z.string().default(""),
    actor_id: z.string().nullable().default(null),
    actor_label: z.string().default(""),
    action: z.string(),
    target_type: z.string().default(""),
    target_id: z.string().default(""),
    target_label: z.string().default(""),
    detail: z.record(z.string(), z.unknown()).default({}),
    ip: z.string().default(""),
    user_agent: z.string().default(""),
    request_id: z.string().default(""),
    created_at: z.string(),
  })
  .loose();

export const AuditEventPageSchema = z
  .object({
    events: z.array(AuditEventSchema).default([]),
    next_cursor: z.string().nullable().default(null),
  })
  .loose();

// An unreadable page is an empty last page: nothing is shown and no cursor is
// offered, so a client cannot loop on a body it cannot read.
export const EMPTY_AUDIT_EVENT_PAGE: AuditEventPage = {
  events: [],
  next_cursor: null,
};
