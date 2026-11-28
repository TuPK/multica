import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { enumOr, listOfItems, nullableStr } from "../schema-helpers";

// Response shapes of the Reach Map routes (durable spec 11, server
// durable_reach_fork.go). Unknown states degrade to "unknown", missing numbers
// read as 0, and malformed surface rows are dropped alone rather than failing
// the whole map.

export const REACH_STATES = [
  "agent_operable",
  "agent_with_confirm",
  "declared_only",
  "human_only",
  "unknown",
] as const;

export const REACH_SURFACE_KINDS = ["app", "channel", "area", "external", "other"] as const;

export type ReachState = (typeof REACH_STATES)[number];
export type ReachSurfaceKind = (typeof REACH_SURFACE_KINDS)[number];

const nonNegNumber = z.number().finite().nonnegative().catch(0).default(0);
const nonNegInt = z.number().int().nonnegative().catch(0).default(0);

export const ReachSurfaceSchema = z
  .object({
    surface: z.string().min(1),
    state: enumOr(REACH_STATES, "unknown"),
    effects_30d: nonNegInt,
    agent_effects_30d: nonNegInt,
    human_minutes_30d: nonNegNumber,
    gap_asks_30d: nonNegInt,
    top_ask_ids: listOfItems(z.string().min(1)),
    gap_issue_id: nullableStr,
  })
  .loose();

export const ReachMapSchema = z
  .object({
    snapshot_date: nullableStr,
    agent_share_30d: nonNegNumber,
    surfaces: listOfItems(ReachSurfaceSchema),
  })
  .loose();

export const CloseReachGapResponseSchema = z
  .object({
    issue_id: z.string().min(1),
  })
  .loose();

export type ReachSurface = z.infer<typeof ReachSurfaceSchema>;
export type ReachMap = z.infer<typeof ReachMapSchema>;
export type CloseReachGapResponse = z.infer<typeof CloseReachGapResponseSchema>;

export const EMPTY_REACH_MAP: ReachMap = {
  snapshot_date: null,
  agent_share_30d: 0,
  surfaces: [],
};

/** Parses GET /api/reach with a safe empty map fallback on malformed payloads. */
export function parseReachMap(raw: unknown, endpoint = "GET /api/reach"): ReachMap {
  return parseWithFallback<ReachMap>(raw, ReachMapSchema, EMPTY_REACH_MAP, { endpoint });
}

/** Parses POST /api/reach/gaps; returns null when the server payload lacks an issue_id. */
export function parseCloseReachGapResponse(
  raw: unknown,
  endpoint = "POST /api/reach/gaps",
): CloseReachGapResponse | null {
  return parseWithFallback<CloseReachGapResponse | null>(raw, CloseReachGapResponseSchema, null, {
    endpoint,
  });
}

/** Extracts the surface kind prefix defined in durable spec 11.2. */
export function reachSurfaceKind(surface: string): ReachSurfaceKind {
  if (surface.startsWith("app:")) return "app";
  if (surface.startsWith("channel:")) return "channel";
  if (surface.startsWith("area:")) return "area";
  if (surface.startsWith("external:")) return "external";
  return "other";
}

/** Strips the known kind prefix from a surface name for compact display. */
export function reachSurfaceLabel(surface: string): string {
  const kind = reachSurfaceKind(surface);
  if (kind === "other") return surface;
  return surface.slice(kind.length + 1) || surface;
}
